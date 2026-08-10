package social

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/lib/pq"
	"github.com/memberclass-backend-golang/internal/shared/constants"
	"github.com/memberclass-backend-golang/internal/shared/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/shared/utils"
)

// ---------- DTOs ----------

type createPostRequest struct {
	UserID string `json:"userId"`
	// TopicID is required when creating; PostID is required when editing.
	TopicID    string  `json:"topicId,omitempty"`
	PostID     string  `json:"postId,omitempty"`
	TenantID   string  `json:"tenantId,omitempty"`
	Title      string  `json:"title"`
	Content    string  `json:"content"`
	Image      *string `json:"image,omitempty"`
	VideoEmbed *string `json:"videoEmbed,omitempty"`
}

type socialCommentResponse struct {
	OK bool   `json:"ok"`
	ID string `json:"id,omitempty"`
}

// topicInfo carries the access rules attached to a topic.
type topicInfo struct {
	ID        string
	OnlyAdmin bool
	// DeliveryIDs, when non-empty, restricts posting to members who own at
	// least one of those deliveries.
	DeliveryIDs []string
}

var (
	errUserNotInTenant  = errors.New("Usuário não encontrado ou não pertence ao tenant autenticado")
	errPostNotFound     = errors.New("Post não encontrado")
	errTopicNotFound    = errors.New("Tópico não existe")
	errPermissionDenied = errors.New("Você não tem autorização para fazer esta ação")
	errNoAccessToTopic  = errors.New("Você não tem acesso para publicar neste tópico")

	errUserIDRequired  = errors.New("userId é obrigatório")
	errTopicIDRequired = errors.New("topicId é obrigatório para criar post")
	errTitleRequired   = errors.New("title é obrigatório")
	errContentRequired = errors.New("content é obrigatório")
)

// ---------- 1. HTTP handler ----------

// CreateOrUpdatePost handles `POST /api/v1/social`.
func (f *Feature) CreateOrUpdatePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req createPostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	tenant := constants.GetTenantFromContext(r.Context())
	if tenant == nil {
		writeCustomError(w, http.StatusUnauthorized, "API key invalid", "INVALID_API_KEY")
		return
	}

	resp, err := f.createOrUpdatePost(r.Context(), req, tenant.ID)
	if err != nil {
		f.writeUseCaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (r *createPostRequest) validate() error {
	if r.UserID == "" {
		return errUserIDRequired
	}
	if r.PostID == "" && r.TopicID == "" {
		return errTopicIDRequired
	}
	if r.Title == "" {
		return errTitleRequired
	}
	if r.Content == "" {
		return errContentRequired
	}
	return nil
}

// ---------- 2. Business rules ----------

func (f *Feature) createOrUpdatePost(ctx context.Context, req createPostRequest, tenantID string) (*socialCommentResponse, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}

	// The author is taken from the body, so membership has to be verified
	// against the authenticated tenant before anything is written.
	belongs, err := f.userBelongsToTenant(ctx, req.UserID, tenantID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, errUserNotInTenant
	}

	if req.PostID != "" {
		return f.updatePost(ctx, req, tenantID)
	}
	return f.createPost(ctx, req, tenantID)
}

// updatePost lets a member edit their own post; tenant owners may edit anyone's.
func (f *Feature) updatePost(ctx context.Context, req createPostRequest, tenantID string) (*socialCommentResponse, error) {
	authorID, err := f.postAuthor(ctx, req.PostID)
	if err != nil {
		return nil, err
	}

	isOwner, err := f.isTenantOwner(ctx, req.UserID, tenantID)
	if err != nil {
		return nil, err
	}

	if !isOwner && authorID != req.UserID {
		return nil, errPermissionDenied
	}

	if err := f.applyPostUpdate(ctx, req); err != nil {
		return nil, err
	}

	return &socialCommentResponse{OK: true, ID: req.PostID}, nil
}

// createPost enforces the topic's access rules: admin-only topics are closed to
// members, and a topic tied to deliveries is open only to members who own one
// of them. Tenant owners bypass both.
func (f *Feature) createPost(ctx context.Context, req createPostRequest, tenantID string) (*socialCommentResponse, error) {
	isOwner, err := f.isTenantOwner(ctx, req.UserID, tenantID)
	if err != nil {
		return nil, err
	}

	topic, err := f.topic(ctx, req.TopicID)
	if err != nil {
		return nil, err
	}
	if topic == nil {
		return nil, errTopicNotFound
	}

	if topic.OnlyAdmin && !isOwner {
		return nil, errNoAccessToTopic
	}

	if len(topic.DeliveryIDs) > 0 && !isOwner {
		userDeliveries, err := f.userDeliveryIDs(ctx, req.UserID, tenantID)
		if err != nil {
			return nil, err
		}
		if !intersects(userDeliveries, topic.DeliveryIDs) {
			return nil, errNoAccessToTopic
		}
	}

	postID, err := f.insertPost(ctx, req)
	if err != nil {
		return nil, err
	}

	return &socialCommentResponse{OK: true, ID: postID}, nil
}

func intersects(a, b []string) bool {
	set := make(map[string]struct{}, len(a))
	for _, v := range a {
		set[v] = struct{}{}
	}
	for _, v := range b {
		if _, ok := set[v]; ok {
			return true
		}
	}
	return false
}

// ---------- 3. SQL ----------

const (
	sqlUserBelongsToTenant = `
		SELECT EXISTS(
			SELECT 1 FROM "UsersOnTenants"
			WHERE "userId" = $1 AND "tenantId" = $2
		)
	`

	sqlIsTenantOwner = `
		SELECT "userId"
		FROM "UsersOnTenants"
		WHERE "userId" = $1 AND "tenantId" = $2 AND role = 'owner'
		LIMIT 1
	`

	sqlUserDeliveryIDs = `
		SELECT "deliveryId"
		FROM "MemberOnDelivery"
		WHERE "memberId" = $1 AND "tenantId" = $2
	`

	// sqlTopic returns the topic's access rules. The FILTER keeps the array
	// empty rather than {NULL} when the topic has no deliveries attached.
	sqlTopic = `
		SELECT t.id, t."onlyAdmin", COALESCE(array_agg(tod."deliveryId") FILTER (WHERE tod."deliveryId" IS NOT NULL), '{}')
		FROM "Topic" t
		LEFT JOIN "TopicOnDelivery" tod ON tod."topicId" = t.id
		WHERE t.id = $1
		GROUP BY t.id, t."onlyAdmin"
	`

	sqlPostAuthor = `SELECT id, "userId" FROM "Post" WHERE id = $1`

	sqlInsertPost = `
		INSERT INTO "Post" (id, "topicId", title, content, published, image, "videoEmbed", "userId", "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, true, $5, $6, $7, NOW(), NOW())
		RETURNING id
	`

	sqlUpdatePost = `
		UPDATE "Post"
		SET title = $2, content = $3, image = $4, "videoEmbed" = $5, "updatedAt" = NOW()
		WHERE id = $1
	`
)

func (f *Feature) userBelongsToTenant(ctx context.Context, userID, tenantID string) (bool, error) {
	var belongs bool
	if err := f.db.QueryRowContext(ctx, sqlUserBelongsToTenant, userID, tenantID).Scan(&belongs); err != nil {
		return false, f.fail("Error checking user tenant membership: ", err, "error checking user tenant membership")
	}
	return belongs, nil
}

func (f *Feature) isTenantOwner(ctx context.Context, userID, tenantID string) (bool, error) {
	var id string
	err := f.db.QueryRowContext(ctx, sqlIsTenantOwner, userID, tenantID).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, f.fail("Error checking if user is owner: ", err, "error checking if user is owner")
	}
	return true, nil
}

func (f *Feature) userDeliveryIDs(ctx context.Context, userID, tenantID string) ([]string, error) {
	rows, err := f.db.QueryContext(ctx, sqlUserDeliveryIDs, userID, tenantID)
	if err != nil {
		return nil, f.fail("Error getting user delivery IDs: ", err, "error getting user delivery IDs")
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, f.fail("Error scanning delivery ID: ", err, "error scanning delivery ID")
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, f.fail("Error iterating delivery IDs: ", err, "error iterating delivery IDs")
	}
	return ids, nil
}

// topic returns nil (without an error) when the topic does not exist.
func (f *Feature) topic(ctx context.Context, topicID string) (*topicInfo, error) {
	var t topicInfo
	var deliveryIDs pq.StringArray

	err := f.db.QueryRowContext(ctx, sqlTopic, topicID).Scan(&t.ID, &t.OnlyAdmin, &deliveryIDs)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, f.fail("Error finding topic: ", err, "error finding topic")
	}

	t.DeliveryIDs = []string(deliveryIDs)
	return &t, nil
}

func (f *Feature) postAuthor(ctx context.Context, postID string) (string, error) {
	var id, userID string
	err := f.db.QueryRowContext(ctx, sqlPostAuthor, postID).Scan(&id, &userID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", errPostNotFound
	case err != nil:
		return "", f.fail("Error finding post: ", err, "error finding post")
	}
	return userID, nil
}

func (f *Feature) insertPost(ctx context.Context, req createPostRequest) (string, error) {
	// The Post id is generated here rather than by the database: the schema is
	// owned by Prisma, whose ids are CUIDs.
	id := utils.GenerateCUID()

	err := f.db.QueryRowContext(ctx, sqlInsertPost,
		id, req.TopicID, req.Title, req.Content, req.Image, req.VideoEmbed, req.UserID).Scan(&id)
	if err != nil {
		return "", f.fail("Error creating post: ", err, "error creating post")
	}
	return id, nil
}

func (f *Feature) applyPostUpdate(ctx context.Context, req createPostRequest) error {
	_, err := f.db.ExecContext(ctx, sqlUpdatePost,
		req.PostID, req.Title, req.Content, req.Image, req.VideoEmbed)
	if err != nil {
		return f.fail("Error updating post: ", err, "error updating post")
	}
	return nil
}

// ---------- errors and responses ----------

func (f *Feature) fail(logPrefix string, err error, message string) error {
	f.log.Error(logPrefix + err.Error())
	return &memberclasserrors.MemberClassError{Code: 500, Message: message}
}

// writeUseCaseError maps each rule failure to the status and error code the
// community frontend switches on.
func (f *Feature) writeUseCaseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errUserNotInTenant):
		writeCustomError(w, http.StatusForbidden, "Usuário não pertence ao tenant", "PERMISSION_DENIED")
	case errors.Is(err, errPostNotFound):
		writeCustomError(w, http.StatusNotFound, "Post não encontrado", "POST_NOT_FOUND")
	case errors.Is(err, errTopicNotFound):
		writeCustomError(w, http.StatusNotFound, "Tópico não existe", "TOPIC_NOT_FOUND")
	case errors.Is(err, errPermissionDenied):
		writeCustomError(w, http.StatusForbidden, "Você não tem autorização para fazer esta ação", "PERMISSION_DENIED")
	case errors.Is(err, errNoAccessToTopic):
		writeCustomError(w, http.StatusForbidden, "Você não tem acesso para publicar neste tópico", "NO_ACCESS_TO_TOPIC")
	case errors.Is(err, errUserIDRequired):
		writeCustomError(w, http.StatusBadRequest, errUserIDRequired.Error(), "MISSING_USER")
	case errors.Is(err, errTopicIDRequired):
		writeCustomError(w, http.StatusBadRequest, errTopicIDRequired.Error(), "MISSING_TOPIC")
	case errors.Is(err, errTitleRequired):
		writeCustomError(w, http.StatusBadRequest, errTitleRequired.Error(), "MISSING_TITLE")
	case errors.Is(err, errContentRequired):
		writeCustomError(w, http.StatusBadRequest, errContentRequired.Error(), "MISSING_CONTENT")
	default:
		var mcErr *memberclasserrors.MemberClassError
		if errors.As(err, &mcErr) {
			writeError(w, mcErr.Code, mcErr.Message)
			return
		}
		f.log.Error("Unexpected error: " + err.Error())
		writeError(w, http.StatusInternalServerError, "Internal server error")
	}
}

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, struct {
		Error   string `json:"error"`
		Message string `json:"message,omitempty"`
	}{Error: http.StatusText(code), Message: message})
}

func writeCustomError(w http.ResponseWriter, code int, message, errorCode string) {
	writeJSON(w, code, map[string]any{
		"ok":        false,
		"error":     message,
		"errorCode": errorCode,
	})
}
