package bunny

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/platform/logger"
	"github.com/memberclass-backend-golang/internal/platform/telemetry"
)

// The account API is a different service from the Stream API this package's
// other client talks to: a different host, and the account key rather than a
// per-library one. It is the only place that answers how much a library has
// stored and how much it has served.
const (
	// StatisticsMaxWindow is the widest date range /statistics accepts. Past
	// it the call answers `statistics.date_range_invalid`, so one call per
	// calendar month is the largest useful unit.
	StatisticsMaxWindow = 40 * 24 * time.Hour

	// StatisticsRetention is how far back /statistics answers at all. Older
	// than this it refuses with "Cannot request statistics older than 1 year",
	// which is why a backfill is worth running early: every month of delay
	// erases a month from the far end for good.
	StatisticsRetention = 365 * 24 * time.Hour

	// statisticsDateFormat is what /statistics parses in dateFrom / dateTo.
	statisticsDateFormat = "2006-01-02"
)

// ErrLibraryNotFound is a 404 from /videolibrary/{id}: the tenant row names a
// library the account no longer has. It is deliberately a distinct error,
// because the caller must record it as unknown usage rather than as zero and
// must not treat it as a failure worth alerting on.
var ErrLibraryNotFound = errors.New("bunny: video library not found")

// ErrUnauthorized is a 401 or 403 from the account API. Unlike a per-library
// 404 this is systemic — one bad account key fails every area — so the caller
// is expected to abort the run rather than carry on failing 121 times.
var ErrUnauthorized = errors.New("bunny: account API key rejected")

// ErrRateLimited is a 429 that survived every retry below. Like ErrUnauthorized
// it is systemic rather than about one area: the account's budget is spent, and
// the next area's call is going to meet the same wall. A caller that keeps
// going in this state does not collect data, it just burns through its areas
// producing nothing — which is exactly what one run of the backfill did, 24
// consecutive months failed across 5 tenants with not a single row written.
var ErrRateLimited = errors.New("bunny: rate limited")

// ErrStatisticsOutOfRange is a range /statistics will not answer: wider than
// StatisticsMaxWindow, or starting before StatisticsRetention. It exists so a
// backfill can tell "Bunny does not know" from "the call failed" — the first
// must leave no row at all, since an absent row reads as "we don't know" and a
// zero reads as "nothing was used".
var ErrStatisticsOutOfRange = errors.New("bunny: statistics date range outside what the API answers")

// VideoLibrary is the subset of GET /videolibrary/{id} the usage worker reads.
//
// The two usage fields are not the same kind of number. TrafficUsage is a flow
// accumulated over the current month, which Bunny itself resets at the UTC turn
// of the 1st; StorageUsage is an instantaneous reading with no history behind
// it anywhere.
type VideoLibrary struct {
	ID           int    `json:"Id"`
	Name         string `json:"Name"`
	PullZoneID   int    `json:"PullZoneId"`
	StorageUsage int64  `json:"StorageUsage"`
	TrafficUsage int64  `json:"TrafficUsage"`
}

// Statistics is the subset of GET /statistics the usage worker reads.
//
// TotalBandwidthUsed already sums the period, so nothing needs to add up
// BandwidthUsedChart. It is the only traffic figure that survives the monthly
// reset, which is what makes it the one a closed month is written from.
type Statistics struct {
	TotalBandwidthUsed int64 `json:"TotalBandwidthUsed"`
}

// AccountService is Bunny's account-level API. It is separate from Service
// because it carries a different credential: the account key from config, held
// by the client, rather than a per-tenant library key passed in per call.
type AccountService interface {
	// GetVideoLibrary reads one library's current storage and month-to-date
	// traffic. It answers ErrLibraryNotFound for a library the account does
	// not have.
	GetVideoLibrary(ctx context.Context, libraryID string) (*VideoLibrary, error)

	// GetStatistics reads the bandwidth a pull zone served over a closed
	// period. from and to are inclusive UTC dates.
	GetStatistics(ctx context.Context, pullZoneID string, from, to time.Time) (*Statistics, error)
}

type accountClient struct {
	client  *http.Client
	baseURL string
	apiKey  string
	log     logger.Logger

	// now exists so a test can pin the retention check.
	now func() time.Time
}

// NewAccountService builds the account-level client. It is constructed even
// when cfg.Bunny.APIKey is empty — the calls then fail with ErrUnauthorized
// rather than the client being nil, so a caller that was wired by mistake says
// what is wrong instead of panicking.
func NewAccountService(cfg *config.Config, log logger.Logger) AccountService {
	return &accountClient{
		client:  telemetry.Client(cfg.Bunny.Timeout),
		baseURL: strings.TrimSuffix(cfg.Bunny.AccountBaseURL, "/"),
		apiKey:  cfg.Bunny.APIKey,
		log:     log,
		now:     time.Now,
	}
}

func (a *accountClient) GetVideoLibrary(ctx context.Context, libraryID string) (*VideoLibrary, error) {
	if libraryID == "" {
		return nil, errors.New("bunny: libraryID is required")
	}

	var library VideoLibrary
	if err := a.get(ctx, a.baseURL+"/videolibrary/"+url.PathEscape(libraryID), &library); err != nil {
		return nil, err
	}
	return &library, nil
}

func (a *accountClient) GetStatistics(ctx context.Context, pullZoneID string, from, to time.Time) (*Statistics, error) {
	if pullZoneID == "" {
		return nil, errors.New("bunny: pullZoneID is required")
	}

	from, to = from.UTC(), to.UTC()
	if to.Before(from) {
		return nil, fmt.Errorf("%w: dateTo %s is before dateFrom %s",
			ErrStatisticsOutOfRange, to.Format(statisticsDateFormat), from.Format(statisticsDateFormat))
	}
	// Both limits are checked here rather than at the call site so every caller
	// gets the same answer, and gets it without spending a request to learn it.
	if to.Sub(from) > StatisticsMaxWindow {
		return nil, fmt.Errorf("%w: %s..%s is wider than %d days",
			ErrStatisticsOutOfRange, from.Format(statisticsDateFormat), to.Format(statisticsDateFormat),
			int(StatisticsMaxWindow.Hours()/24))
	}
	if a.now().UTC().Sub(from) > StatisticsRetention {
		return nil, fmt.Errorf("%w: %s is more than a year ago",
			ErrStatisticsOutOfRange, from.Format(statisticsDateFormat))
	}

	query := url.Values{
		"pullZone": []string{pullZoneID},
		"dateFrom": []string{from.Format(statisticsDateFormat)},
		"dateTo":   []string{to.Format(statisticsDateFormat)},
	}

	var stats Statistics
	if err := a.get(ctx, a.baseURL+"/statistics?"+query.Encode(), &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

// maxErrorBody bounds what an error message quotes back. Bunny's error bodies
// are short; an HTML error page from something in front of it is not.
const maxErrorBody = 512

const (
	// rateLimitRetries is how many times a 429 is waited out before the caller
	// is told. Bunny's limit is per account and shared with every other thing
	// touching it, so a burst is worth waiting through rather than failing on.
	rateLimitRetries = 4

	// rateLimitBaseDelay is the first wait, doubled per attempt: 2s, 4s, 8s,
	// 16s — 30s in total, well past the width of an ordinary burst. `Retry-After`
	// wins over it whenever Bunny sends one.
	rateLimitBaseDelay = 2 * time.Second

	// maxRetryAfter caps what a Retry-After can ask for. The header is a value
	// the server chooses, and a run should not be parked for an hour by one.
	maxRetryAfter = 60 * time.Second
)

// get performs one read, waiting out a 429 rather than reporting it.
//
// Without the wait a rate-limited run degenerates: the caller's own throttle is
// the only thing pacing it, a 429 comes back faster than a real response, and
// the run accelerates into the wall it just hit. Backing off is what turns a
// burst into a pause instead of a hole in the data.
func (a *accountClient) get(ctx context.Context, endpoint string, into any) error {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("AccessKey", a.apiKey)
		req.Header.Set("Accept", "application/json")

		resp, err := a.client.Do(req)
		if err != nil {
			return fmt.Errorf("bunny account api: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			wait := retryAfter(resp, rateLimitBaseDelay<<attempt)
			resp.Body.Close()

			if attempt >= rateLimitRetries {
				return fmt.Errorf("%w: gave up after %d attempts", ErrRateLimited, attempt+1)
			}
			a.log.Warn("Bunny rate limited, backing off",
				"endpoint", endpoint, "attempt", attempt+1, "wait", wait.String())

			if err := a.sleep(ctx, wait); err != nil {
				return err
			}
			continue
		}

		err = decode(resp, into)
		resp.Body.Close()
		return err
	}
}

func decode(resp *http.Response, into any) error {
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrLibraryNotFound
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return ErrUnauthorized
	case resp.StatusCode != http.StatusOK:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		return fmt.Errorf("bunny account api status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return fmt.Errorf("decode bunny account response: %w", err)
	}
	return nil
}

// retryAfter prefers what the server asked for over the caller's own guess. It
// accepts only the delay-seconds form; Bunny does not send the HTTP-date one,
// and a date parsed wrong would be worse than the exponential fallback.
func retryAfter(resp *http.Response, fallback time.Duration) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After")))
	if err != nil || seconds <= 0 {
		return fallback
	}
	if wait := time.Duration(seconds) * time.Second; wait <= maxRetryAfter {
		return wait
	}
	return maxRetryAfter
}

// sleep waits, but gives up the moment the run is cancelled — a shutdown should
// not have to sit through a 16-second backoff.
func (a *accountClient) sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
