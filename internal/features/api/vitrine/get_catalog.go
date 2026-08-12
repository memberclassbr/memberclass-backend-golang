package vitrine

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/memberclass-backend-golang/internal/shared/tenant"
)

type vitrineResponse struct {
	Vitrines []vitrineData `json:"vitrines"`
	Total    int           `json:"total"`
}

// ---------- 1. HTTP handler ----------

// GetVitrines handles `GET /api/v1/vitrine`: the tenant's whole catalog, with
// every course, section, module and published lesson inlined.
func (f *Feature) GetVitrines(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	tenant := tenant.FromContext(r.Context())
	if tenant == nil {
		writeCustomError(w, http.StatusUnauthorized, "Token de API inválido", "INVALID_API_KEY")
		return
	}

	resp, err := f.getCatalog(r.Context(), tenant.ID)
	if err != nil {
		f.writeUseCaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// ---------- 2. Business rule ----------

func (f *Feature) getCatalog(ctx context.Context, tenantID string) (*vitrineResponse, error) {
	vitrines, err := f.queryVitrines(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	if len(vitrines) == 0 {
		return &vitrineResponse{Vitrines: []vitrineData{}, Total: 0}, nil
	}

	refs := make([]*vitrineData, len(vitrines))
	for i := range vitrines {
		refs[i] = &vitrines[i]
	}
	if err := f.fillVitrines(ctx, refs); err != nil {
		return nil, err
	}

	return &vitrineResponse{Vitrines: vitrines, Total: len(vitrines)}, nil
}

// ---------- 3. SQL ----------

func (f *Feature) queryVitrines(ctx context.Context, tenantID string) ([]vitrineData, error) {
	rows, err := f.db.QueryContext(ctx, sqlVitrinesByTenant, tenantID)
	if err != nil {
		return nil, f.fail("Error querying vitrines: ", err, "erro ao buscar catálogo")
	}
	defer rows.Close()

	vitrines := make([]vitrineData, 0)
	for rows.Next() {
		var v vitrineData
		var order sql.NullInt32

		// A row that fails to scan is logged and skipped rather than failing
		// the whole catalog — one bad row should not blank a storefront.
		if err := rows.Scan(&v.ID, &v.Name, &v.Published, &order); err != nil {
			f.log.Error("Error scanning vitrine: " + err.Error())
			continue
		}
		v.Order = intPtr(order)
		vitrines = append(vitrines, v)
	}
	return vitrines, nil
}
