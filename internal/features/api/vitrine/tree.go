package vitrine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/lib/pq"
	"github.com/memberclass-backend-golang/internal/shared/memberclasserrors"
)

// ---------- DTOs ----------
//
// `omitempty` on the child slices is part of the contract: a node with no
// children omits the key entirely rather than sending an empty array.

type vitrineData struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Published bool         `json:"published"`
	Order     *int         `json:"order,omitempty"`
	Courses   []courseData `json:"courses,omitempty"`
}

type courseData struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Published bool          `json:"published"`
	Order     *int          `json:"order,omitempty"`
	Sections  []sectionData `json:"sections,omitempty"`
}

type sectionData struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Order   *int         `json:"order,omitempty"`
	Modules []moduleData `json:"modules,omitempty"`
}

type moduleData struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Published bool         `json:"published"`
	Order     *int         `json:"order,omitempty"`
	Lessons   []lessonData `json:"lessons,omitempty"`
}

type lessonData struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Published bool    `json:"published"`
	Slug      *string `json:"slug,omitempty"`
	Type      *string `json:"type,omitempty"`
	MediaURL  *string `json:"mediaUrl,omitempty"`
	Thumbnail *string `json:"thumbnail,omitempty"`
	Order     *int    `json:"order,omitempty"`
}

// ---------- subtree loading ----------
//
// Each level is one query over the ids of the level above. The parent ids are
// always the result of a tenant-scoped query, so the child queries inherit
// that scope without repeating the four-table join up to Vitrine.

func (f *Feature) fillVitrines(ctx context.Context, vitrines []*vitrineData) error {
	ids := make([]string, 0, len(vitrines))
	for _, v := range vitrines {
		ids = append(ids, v.ID)
	}
	if len(ids) == 0 {
		return nil
	}

	byVitrine, err := f.queryCourses(ctx, ids)
	if err != nil {
		return err
	}

	var courses []*courseData
	for _, v := range vitrines {
		v.Courses = byVitrine[v.ID]
		for i := range v.Courses {
			courses = append(courses, &v.Courses[i])
		}
	}

	return f.fillCourses(ctx, courses)
}

func (f *Feature) fillCourses(ctx context.Context, courses []*courseData) error {
	ids := make([]string, 0, len(courses))
	for _, c := range courses {
		ids = append(ids, c.ID)
	}
	if len(ids) == 0 {
		return nil
	}

	byCourse, err := f.querySections(ctx, ids)
	if err != nil {
		return err
	}

	var sections []*sectionData
	for _, c := range courses {
		c.Sections = byCourse[c.ID]
		for i := range c.Sections {
			sections = append(sections, &c.Sections[i])
		}
	}

	return f.fillSections(ctx, sections)
}

func (f *Feature) fillSections(ctx context.Context, sections []*sectionData) error {
	ids := make([]string, 0, len(sections))
	for _, s := range sections {
		ids = append(ids, s.ID)
	}
	if len(ids) == 0 {
		return nil
	}

	bySection, err := f.queryModules(ctx, ids)
	if err != nil {
		return err
	}

	var modules []*moduleData
	for _, s := range sections {
		s.Modules = bySection[s.ID]
		for i := range s.Modules {
			modules = append(modules, &s.Modules[i])
		}
	}

	return f.fillModules(ctx, modules)
}

func (f *Feature) fillModules(ctx context.Context, modules []*moduleData) error {
	ids := make([]string, 0, len(modules))
	for _, m := range modules {
		ids = append(ids, m.ID)
	}
	if len(ids) == 0 {
		return nil
	}

	byModule, err := f.queryLessons(ctx, ids)
	if err != nil {
		return err
	}

	for _, m := range modules {
		m.Lessons = byModule[m.ID]
	}
	return nil
}

// ---------- SQL ----------

const (
	sqlVitrinesByTenant = `
		SELECT v.id, v.name, v.published, v."order"
		FROM "Vitrine" v
		WHERE v."tenantId" = $1
		ORDER BY COALESCE(v."order", 0) ASC
	`

	sqlVitrineByID = `
		SELECT v.id, v.name, v.published, v."order"
		FROM "Vitrine" v
		WHERE v.id = $1 AND v."tenantId" = $2
	`

	sqlCoursesByVitrine = `
		SELECT c.id, c.name, c.published, c."order", c."vitrineId"
		FROM "Course" c
		WHERE c."vitrineId" = ANY($1)
		ORDER BY COALESCE(c."order", 0) ASC
	`

	sqlCourseByID = `
		SELECT c.id, c.name, c.published, c."order"
		FROM "Course" c
		JOIN "Vitrine" v ON c."vitrineId" = v.id
		WHERE c.id = $1 AND v."tenantId" = $2
	`

	sqlSectionsByCourse = `
		SELECT s.id, s.name, s."order", s."courseId"
		FROM "Section" s
		WHERE s."courseId" = ANY($1)
		ORDER BY COALESCE(s."order", 0) ASC
	`

	sqlModulesBySection = `
		SELECT m.id, m.name, m.published, m."order", m."sectionId"
		FROM "Module" m
		WHERE m."sectionId" = ANY($1)
		ORDER BY COALESCE(m."order", 0) ASC
	`

	sqlModuleByID = `
		SELECT m.id, m.name, m.published, m."order"
		FROM "Module" m
		JOIN "Section" s ON m."sectionId" = s.id
		JOIN "Course" c ON s."courseId" = c.id
		JOIN "Vitrine" v ON c."vitrineId" = v.id
		WHERE m.id = $1 AND v."tenantId" = $2
	`

	// Unpublished lessons never reach the catalog, at any level.
	sqlLessonsByModule = `
		SELECT l.id, l.name, l.published, l.slug, l.type, l."mediaUrl", l.thumbnail, l."order", l."moduleId"
		FROM "Lesson" l
		WHERE l."moduleId" = ANY($1) AND l.published = true
		ORDER BY COALESCE(l."order", 0) ASC
	`

	sqlLessonByID = `
		SELECT l.id, l.name, l.published, l.slug, l.type, l."mediaUrl", l.thumbnail, l."order"
		FROM "Lesson" l
		JOIN "Module" m ON l."moduleId" = m.id
		JOIN "Section" s ON m."sectionId" = s.id
		JOIN "Course" c ON s."courseId" = c.id
		JOIN "Vitrine" v ON c."vitrineId" = v.id
		WHERE l.id = $1 AND v."tenantId" = $2
	`
)

func (f *Feature) queryCourses(ctx context.Context, vitrineIDs []string) (map[string][]courseData, error) {
	rows, err := f.db.QueryContext(ctx, sqlCoursesByVitrine, pq.Array(vitrineIDs))
	if err != nil {
		return nil, f.fail("Error querying courses: ", err, "erro ao buscar cursos")
	}
	defer rows.Close()

	byParent := make(map[string][]courseData)
	for rows.Next() {
		var c courseData
		var order sql.NullInt32
		var vitrineID string

		if err := rows.Scan(&c.ID, &c.Name, &c.Published, &order, &vitrineID); err != nil {
			f.log.Error("Error scanning course: " + err.Error())
			continue
		}
		c.Order = intPtr(order)
		byParent[vitrineID] = append(byParent[vitrineID], c)
	}
	return byParent, nil
}

func (f *Feature) querySections(ctx context.Context, courseIDs []string) (map[string][]sectionData, error) {
	rows, err := f.db.QueryContext(ctx, sqlSectionsByCourse, pq.Array(courseIDs))
	if err != nil {
		return nil, f.fail("Error querying sections: ", err, "erro ao buscar seções")
	}
	defer rows.Close()

	byParent := make(map[string][]sectionData)
	for rows.Next() {
		var s sectionData
		var order sql.NullInt32
		var courseID string

		if err := rows.Scan(&s.ID, &s.Name, &order, &courseID); err != nil {
			f.log.Error("Error scanning section: " + err.Error())
			continue
		}
		s.Order = intPtr(order)
		byParent[courseID] = append(byParent[courseID], s)
	}
	return byParent, nil
}

func (f *Feature) queryModules(ctx context.Context, sectionIDs []string) (map[string][]moduleData, error) {
	rows, err := f.db.QueryContext(ctx, sqlModulesBySection, pq.Array(sectionIDs))
	if err != nil {
		return nil, f.fail("Error querying modules: ", err, "erro ao buscar módulos")
	}
	defer rows.Close()

	byParent := make(map[string][]moduleData)
	for rows.Next() {
		var m moduleData
		var order sql.NullInt32
		var sectionID string

		if err := rows.Scan(&m.ID, &m.Name, &m.Published, &order, &sectionID); err != nil {
			f.log.Error("Error scanning module: " + err.Error())
			continue
		}
		m.Order = intPtr(order)
		byParent[sectionID] = append(byParent[sectionID], m)
	}
	return byParent, nil
}

func (f *Feature) queryLessons(ctx context.Context, moduleIDs []string) (map[string][]lessonData, error) {
	rows, err := f.db.QueryContext(ctx, sqlLessonsByModule, pq.Array(moduleIDs))
	if err != nil {
		return nil, f.fail("Error querying lessons: ", err, "erro ao buscar aulas")
	}
	defer rows.Close()

	byParent := make(map[string][]lessonData)
	for rows.Next() {
		var l lessonData
		var slug, lessonType, mediaURL, thumbnail sql.NullString
		var order sql.NullInt32
		var moduleID string

		if err := rows.Scan(&l.ID, &l.Name, &l.Published, &slug, &lessonType, &mediaURL, &thumbnail, &order, &moduleID); err != nil {
			f.log.Error("Error scanning lesson: " + err.Error())
			continue
		}
		applyLessonNulls(&l, slug, lessonType, mediaURL, thumbnail, order)
		byParent[moduleID] = append(byParent[moduleID], l)
	}
	return byParent, nil
}

// ---------- scanning helpers ----------

func intPtr(v sql.NullInt32) *int {
	if !v.Valid {
		return nil
	}
	out := int(v.Int32)
	return &out
}

func strPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	out := v.String
	return &out
}

func applyLessonNulls(l *lessonData, slug, lessonType, mediaURL, thumbnail sql.NullString, order sql.NullInt32) {
	l.Slug = strPtr(slug)
	l.Type = strPtr(lessonType)
	l.MediaURL = strPtr(mediaURL)
	l.Thumbnail = strPtr(thumbnail)
	l.Order = intPtr(order)
}

// ---------- errors and responses ----------

// fail logs the driver error and returns the 500 the client sees.
func (f *Feature) fail(logPrefix string, err error, message string) error {
	f.log.Error(logPrefix + err.Error())
	return &memberclasserrors.MemberClassError{Code: 500, Message: message}
}

// notFoundOr maps a QueryRow failure: no rows becomes the given 404, anything
// else becomes a 500 with the given message.
func (f *Feature) notFoundOr(err error, logPrefix, notFoundMsg, failMsg string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return &memberclasserrors.MemberClassError{Code: 404, Message: notFoundMsg}
	}
	return f.fail(logPrefix, err, failMsg)
}

// parseIncludeChildren reads the ?includeChildren flag. Anything that is not a
// valid boolean means false, matching the previous handler.
func parseIncludeChildren(r *http.Request) bool {
	raw := r.URL.Query().Get("includeChildren")
	if raw == "" {
		return false
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false
	}
	return value
}

func (f *Feature) writeUseCaseError(w http.ResponseWriter, err error) {
	var mcErr *memberclasserrors.MemberClassError
	if !errors.As(err, &mcErr) || mcErr == nil {
		f.log.Error("Unexpected error: " + err.Error())
		writeError(w, http.StatusInternalServerError, "Erro interno do servidor")
		return
	}

	switch mcErr.Code {
	case http.StatusBadRequest:
		writeCustomError(w, http.StatusBadRequest, mcErr.Message, "INVALID_REQUEST")
	case http.StatusUnauthorized:
		writeCustomError(w, http.StatusUnauthorized, mcErr.Message, "INVALID_API_KEY")
	case http.StatusNotFound:
		writeCustomError(w, http.StatusNotFound, mcErr.Message, "NOT_FOUND")
	default:
		writeError(w, mcErr.Code, mcErr.Message)
	}
}

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

// writeError uses the {error, message} shape this endpoint family has always
// returned for 405 and unmapped codes. It differs from writeCustomError on
// purpose — clients parse both.
func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]string{
		"error":   http.StatusText(code),
		"message": message,
	})
}

func writeCustomError(w http.ResponseWriter, code int, message, errorCode string) {
	writeJSON(w, code, map[string]any{
		"ok":        false,
		"error":     message,
		"errorCode": errorCode,
	})
}
