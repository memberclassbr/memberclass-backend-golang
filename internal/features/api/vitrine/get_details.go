package vitrine

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/memberclass-backend-golang/internal/shared/constants"
)

type vitrineDetailResponse struct {
	Vitrine vitrineData `json:"vitrine"`
}

type courseDetailResponse struct {
	Course courseData `json:"course"`
}

type moduleDetailResponse struct {
	Module moduleData `json:"module"`
}

type lessonDetailResponse struct {
	Lesson lessonData `json:"lesson"`
}

// ---------- 1. HTTP handlers ----------
//
// The four lookups share a shape: require GET, require the path id, require a
// tenant, then delegate. resolve() collapses that preamble; each handler is
// left with the part that differs.

// resolve validates the request and returns the path id and tenant id. It
// writes the response and returns ok=false when the request is rejected.
func (f *Feature) resolve(w http.ResponseWriter, r *http.Request, param string) (id, tenantID string, ok bool) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return "", "", false
	}

	id = chi.URLParam(r, param)
	if id == "" {
		writeCustomError(w, http.StatusBadRequest, param+" é obrigatório", "INVALID_REQUEST")
		return "", "", false
	}

	tenant := constants.GetTenantFromContext(r.Context())
	if tenant == nil {
		writeCustomError(w, http.StatusUnauthorized, "Token de API inválido", "INVALID_API_KEY")
		return "", "", false
	}

	return id, tenant.ID, true
}

// GetVitrine handles `GET /api/v1/vitrine/{vitrineId}`.
func (f *Feature) GetVitrine(w http.ResponseWriter, r *http.Request) {
	vitrineID, tenantID, ok := f.resolve(w, r, "vitrineId")
	if !ok {
		return
	}

	resp, err := f.getVitrine(r.Context(), vitrineID, tenantID, parseIncludeChildren(r))
	if err != nil {
		f.writeUseCaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetCourse handles `GET /api/v1/vitrine/courses/{courseId}`.
func (f *Feature) GetCourse(w http.ResponseWriter, r *http.Request) {
	courseID, tenantID, ok := f.resolve(w, r, "courseId")
	if !ok {
		return
	}

	resp, err := f.getCourse(r.Context(), courseID, tenantID, parseIncludeChildren(r))
	if err != nil {
		f.writeUseCaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetModule handles `GET /api/v1/vitrine/modules/{moduleId}`.
func (f *Feature) GetModule(w http.ResponseWriter, r *http.Request) {
	moduleID, tenantID, ok := f.resolve(w, r, "moduleId")
	if !ok {
		return
	}

	resp, err := f.getModule(r.Context(), moduleID, tenantID, parseIncludeChildren(r))
	if err != nil {
		f.writeUseCaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetLesson handles `GET /api/v1/vitrine/lessons/{lessonId}`. A lesson is a
// leaf, so it takes no includeChildren.
func (f *Feature) GetLesson(w http.ResponseWriter, r *http.Request) {
	lessonID, tenantID, ok := f.resolve(w, r, "lessonId")
	if !ok {
		return
	}

	resp, err := f.getLesson(r.Context(), lessonID, tenantID)
	if err != nil {
		f.writeUseCaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---------- 2. Business rules ----------

func (f *Feature) getVitrine(ctx context.Context, vitrineID, tenantID string, includeChildren bool) (*vitrineDetailResponse, error) {
	var v vitrineData
	var order sql.NullInt32

	err := f.db.QueryRowContext(ctx, sqlVitrineByID, vitrineID, tenantID).
		Scan(&v.ID, &v.Name, &v.Published, &order)
	if err != nil {
		return nil, f.notFoundOr(err, "Error querying vitrine: ", "Vitrine não encontrada", "erro ao buscar vitrine")
	}
	v.Order = intPtr(order)

	if includeChildren {
		if err := f.fillVitrines(ctx, []*vitrineData{&v}); err != nil {
			return nil, err
		}
	}

	return &vitrineDetailResponse{Vitrine: v}, nil
}

func (f *Feature) getCourse(ctx context.Context, courseID, tenantID string, includeChildren bool) (*courseDetailResponse, error) {
	var c courseData
	var order sql.NullInt32

	err := f.db.QueryRowContext(ctx, sqlCourseByID, courseID, tenantID).
		Scan(&c.ID, &c.Name, &c.Published, &order)
	if err != nil {
		return nil, f.notFoundOr(err, "Error querying course: ", "Curso não encontrado", "erro ao buscar curso")
	}
	c.Order = intPtr(order)

	if includeChildren {
		if err := f.fillCourses(ctx, []*courseData{&c}); err != nil {
			return nil, err
		}
	}

	return &courseDetailResponse{Course: c}, nil
}

func (f *Feature) getModule(ctx context.Context, moduleID, tenantID string, includeChildren bool) (*moduleDetailResponse, error) {
	var m moduleData
	var order sql.NullInt32

	err := f.db.QueryRowContext(ctx, sqlModuleByID, moduleID, tenantID).
		Scan(&m.ID, &m.Name, &m.Published, &order)
	if err != nil {
		return nil, f.notFoundOr(err, "Error querying module: ", "Módulo não encontrado", "erro ao buscar módulo")
	}
	m.Order = intPtr(order)

	if includeChildren {
		if err := f.fillModules(ctx, []*moduleData{&m}); err != nil {
			return nil, err
		}
	}

	return &moduleDetailResponse{Module: m}, nil
}

func (f *Feature) getLesson(ctx context.Context, lessonID, tenantID string) (*lessonDetailResponse, error) {
	var l lessonData
	var slug, lessonType, mediaURL, thumbnail sql.NullString
	var order sql.NullInt32

	err := f.db.QueryRowContext(ctx, sqlLessonByID, lessonID, tenantID).
		Scan(&l.ID, &l.Name, &l.Published, &slug, &lessonType, &mediaURL, &thumbnail, &order)
	if err != nil {
		return nil, f.notFoundOr(err, "Error querying lesson: ", "Aula não encontrada", "erro ao buscar aula")
	}
	applyLessonNulls(&l, slug, lessonType, mediaURL, thumbnail, order)

	return &lessonDetailResponse{Lesson: l}, nil
}
