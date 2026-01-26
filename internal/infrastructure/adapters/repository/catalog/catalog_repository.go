package catalog

import (
	"context"
	"database/sql"
	"errors"

	"github.com/memberclass-backend-golang/internal/domain/dto/response/catalog"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	catalogports "github.com/memberclass-backend-golang/internal/domain/ports/catalog"
)

type CatalogRepository struct {
	db  *sql.DB
	log ports.Logger
}

func NewCatalogRepository(db *sql.DB, log ports.Logger) catalogports.CatalogRepository {
	return &CatalogRepository{
		db:  db,
		log: log,
	}
}

func (r *CatalogRepository) GetCatalogByTenant(ctx context.Context, tenantID string) (*catalog.CatalogResponse, error) {
	query := `
		SELECT 
			v.id,
			v.name,
			v."order"
		FROM "Vitrine" v
		WHERE v."tenantId" = $1
		ORDER BY COALESCE(v."order", 0) ASC
	`

	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		r.log.Error("Error querying vitrines: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "erro ao buscar catálogo",
		}
	}
	defer rows.Close()

	vitrinesMap := make(map[string]*catalog.VitrineData)
	var vitrines []*catalog.VitrineData

	for rows.Next() {
		var vitrine catalog.VitrineData
		var order sql.NullInt32

		err := rows.Scan(&vitrine.ID, &vitrine.Name, &order)
		if err != nil {
			r.log.Error("Error scanning vitrine: " + err.Error())
			continue
		}

		if order.Valid {
			orderVal := int(order.Int32)
			vitrine.Order = &orderVal
		}

		vitrine.Courses = []catalog.CourseData{}
		vitrinesMap[vitrine.ID] = &vitrine
		vitrines = append(vitrines, &vitrine)
	}

	if len(vitrines) == 0 {
		return &catalog.CatalogResponse{
			Vitrines: []catalog.VitrineData{},
			Total:    0,
		}, nil
	}

	coursesQuery := `
		SELECT 
			c.id,
			c.name,
			c."order",
			c."vitrineId"
		FROM "Course" c
		JOIN "Vitrine" v ON c."vitrineId" = v.id
		WHERE v."tenantId" = $1
		ORDER BY COALESCE(c."order", 0) ASC
	`

	coursesRows, err := r.db.QueryContext(ctx, coursesQuery, tenantID)
	if err != nil {
		r.log.Error("Error querying courses: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "erro ao buscar cursos",
		}
	}
	defer coursesRows.Close()

	coursesMap := make(map[string]*catalog.CourseData)

	for coursesRows.Next() {
		var course catalog.CourseData
		var order sql.NullInt32
		var vitrineID string

		err := coursesRows.Scan(&course.ID, &course.Name, &order, &vitrineID)
		if err != nil {
			r.log.Error("Error scanning course: " + err.Error())
			continue
		}

		if order.Valid {
			orderVal := int(order.Int32)
			course.Order = &orderVal
		}

		course.Modules = []catalog.ModuleData{}
		coursesMap[course.ID] = &course

		if vitrine, ok := vitrinesMap[vitrineID]; ok {
			vitrine.Courses = append(vitrine.Courses, course)
		}
	}

	modulesQuery := `
		SELECT 
			m.id,
			m.name,
			m."order",
			s."courseId"
		FROM "Module" m
		JOIN "Section" s ON m."sectionId" = s.id
		JOIN "Course" c ON s."courseId" = c.id
		JOIN "Vitrine" v ON c."vitrineId" = v.id
		WHERE v."tenantId" = $1
		ORDER BY COALESCE(m."order", 0) ASC
	`

	modulesRows, err := r.db.QueryContext(ctx, modulesQuery, tenantID)
	if err != nil {
		r.log.Error("Error querying modules: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "erro ao buscar módulos",
		}
	}
	defer modulesRows.Close()

	modulesMap := make(map[string]*catalog.ModuleData)

	for modulesRows.Next() {
		var module catalog.ModuleData
		var order sql.NullInt32
		var courseID string

		err := modulesRows.Scan(&module.ID, &module.Name, &order, &courseID)
		if err != nil {
			r.log.Error("Error scanning module: " + err.Error())
			continue
		}

		if order.Valid {
			orderVal := int(order.Int32)
			module.Order = &orderVal
		}

		module.Lessons = []catalog.LessonData{}
		modulesMap[module.ID] = &module

		for i := range vitrines {
			for j := range vitrines[i].Courses {
				if vitrines[i].Courses[j].ID == courseID {
					vitrines[i].Courses[j].Modules = append(vitrines[i].Courses[j].Modules, module)
					break
				}
			}
		}
	}

	lessonsQuery := `
		SELECT 
			l.id,
			l.name,
			l.slug,
			l.type,
			l."mediaUrl",
			l.thumbnail,
			l."order",
			l."moduleId"
		FROM "Lesson" l
		JOIN "Module" m ON l."moduleId" = m.id
		JOIN "Section" s ON m."sectionId" = s.id
		JOIN "Course" c ON s."courseId" = c.id
		JOIN "Vitrine" v ON c."vitrineId" = v.id
		WHERE v."tenantId" = $1
			AND l.published = true
		ORDER BY COALESCE(l."order", 0) ASC
	`

	lessonsRows, err := r.db.QueryContext(ctx, lessonsQuery, tenantID)
	if err != nil {
		r.log.Error("Error querying lessons: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "erro ao buscar aulas",
		}
	}
	defer lessonsRows.Close()

	for lessonsRows.Next() {
		var lesson catalog.LessonData
		var slug, lessonType, mediaURL, thumbnail sql.NullString
		var order sql.NullInt32
		var moduleID string

		err := lessonsRows.Scan(&lesson.ID, &lesson.Name, &slug, &lessonType, &mediaURL, &thumbnail, &order, &moduleID)
		if err != nil {
			r.log.Error("Error scanning lesson: " + err.Error())
			continue
		}

		if slug.Valid {
			lesson.Slug = &slug.String
		}
		if lessonType.Valid {
			lesson.Type = &lessonType.String
		}
		if mediaURL.Valid {
			lesson.MediaURL = &mediaURL.String
		}
		if thumbnail.Valid {
			lesson.Thumbnail = &thumbnail.String
		}
		if order.Valid {
			orderVal := int(order.Int32)
			lesson.Order = &orderVal
		}

		for i := range vitrines {
			for j := range vitrines[i].Courses {
				for k := range vitrines[i].Courses[j].Modules {
					if vitrines[i].Courses[j].Modules[k].ID == moduleID {
						vitrines[i].Courses[j].Modules[k].Lessons = append(vitrines[i].Courses[j].Modules[k].Lessons, lesson)
						break
					}
				}
			}
		}
	}

	result := make([]catalog.VitrineData, len(vitrines))
	for i, v := range vitrines {
		result[i] = *v
	}

	return &catalog.CatalogResponse{
		Vitrines: result,
		Total:    len(result),
	}, nil
}

func (r *CatalogRepository) GetVitrineByID(ctx context.Context, vitrineID, tenantID string, includeChildren bool) (*catalog.VitrineDetailResponse, error) {
	query := `
		SELECT 
			v.id,
			v.name,
			v."order"
		FROM "Vitrine" v
		WHERE v.id = $1 AND v."tenantId" = $2
	`

	var vitrine catalog.VitrineData
	var order sql.NullInt32

	err := r.db.QueryRowContext(ctx, query, vitrineID, tenantID).Scan(&vitrine.ID, &vitrine.Name, &order)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "Vitrine não encontrada",
			}
		}
		r.log.Error("Error querying vitrine: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "erro ao buscar vitrine",
		}
	}

	if order.Valid {
		orderVal := int(order.Int32)
		vitrine.Order = &orderVal
	}

	if includeChildren {
		coursesQuery := `
			SELECT 
				c.id,
				c.name,
				c."order"
			FROM "Course" c
			WHERE c."vitrineId" = $1
			ORDER BY COALESCE(c."order", 0) ASC
		`

		coursesRows, err := r.db.QueryContext(ctx, coursesQuery, vitrineID)
		if err != nil {
			r.log.Error("Error querying courses: " + err.Error())
			return nil, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "erro ao buscar cursos",
			}
		}
		defer coursesRows.Close()

		for coursesRows.Next() {
			var course catalog.CourseData
			var order sql.NullInt32

			err := coursesRows.Scan(&course.ID, &course.Name, &order)
			if err != nil {
				r.log.Error("Error scanning course: " + err.Error())
				continue
			}

			if order.Valid {
				orderVal := int(order.Int32)
				course.Order = &orderVal
			}

			modulesQuery := `
				SELECT 
					m.id,
					m.name,
					m."order"
				FROM "Module" m
				JOIN "Section" s ON m."sectionId" = s.id
				WHERE s."courseId" = $1
				ORDER BY COALESCE(m."order", 0) ASC
			`

			modulesRows, err := r.db.QueryContext(ctx, modulesQuery, course.ID)
			if err != nil {
				r.log.Error("Error querying modules: " + err.Error())
				continue
			}

			for modulesRows.Next() {
				var module catalog.ModuleData
				var order sql.NullInt32

				err := modulesRows.Scan(&module.ID, &module.Name, &order)
				if err != nil {
					r.log.Error("Error scanning module: " + err.Error())
					modulesRows.Close()
					continue
				}

				if order.Valid {
					orderVal := int(order.Int32)
					module.Order = &orderVal
				}

				lessonsQuery := `
					SELECT 
						l.id,
						l.name,
						l.slug,
						l.type,
						l."mediaUrl",
						l.thumbnail,
						l."order"
					FROM "Lesson" l
					WHERE l."moduleId" = $1 AND l.published = true
					ORDER BY COALESCE(l."order", 0) ASC
				`

				lessonsRows, err := r.db.QueryContext(ctx, lessonsQuery, module.ID)
				if err != nil {
					r.log.Error("Error querying lessons: " + err.Error())
					modulesRows.Close()
					continue
				}

				for lessonsRows.Next() {
					var lesson catalog.LessonData
					var slug, lessonType, mediaURL, thumbnail sql.NullString
					var order sql.NullInt32

					err := lessonsRows.Scan(&lesson.ID, &lesson.Name, &slug, &lessonType, &mediaURL, &thumbnail, &order)
					if err != nil {
						r.log.Error("Error scanning lesson: " + err.Error())
						continue
					}

					if slug.Valid {
						lesson.Slug = &slug.String
					}
					if lessonType.Valid {
						lesson.Type = &lessonType.String
					}
					if mediaURL.Valid {
						lesson.MediaURL = &mediaURL.String
					}
					if thumbnail.Valid {
						lesson.Thumbnail = &thumbnail.String
					}
					if order.Valid {
						orderVal := int(order.Int32)
						lesson.Order = &orderVal
					}

					module.Lessons = append(module.Lessons, lesson)
				}
				lessonsRows.Close()

				course.Modules = append(course.Modules, module)
			}
			modulesRows.Close()

			vitrine.Courses = append(vitrine.Courses, course)
		}
	} else {
		vitrine.Courses = []catalog.CourseData{}
	}

	return &catalog.VitrineDetailResponse{
		Vitrine: vitrine,
	}, nil
}

func (r *CatalogRepository) GetCourseByID(ctx context.Context, courseID, tenantID string, includeChildren bool) (*catalog.CourseDetailResponse, error) {
	query := `
		SELECT 
			c.id,
			c.name,
			c."order"
		FROM "Course" c
		JOIN "Vitrine" v ON c."vitrineId" = v.id
		WHERE c.id = $1 AND v."tenantId" = $2
	`

	var course catalog.CourseData
	var order sql.NullInt32

	err := r.db.QueryRowContext(ctx, query, courseID, tenantID).Scan(&course.ID, &course.Name, &order)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "Curso não encontrado",
			}
		}
		r.log.Error("Error querying course: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "erro ao buscar curso",
		}
	}

	if order.Valid {
		orderVal := int(order.Int32)
		course.Order = &orderVal
	}

	if includeChildren {
		modulesQuery := `
			SELECT 
				m.id,
				m.name,
				m."order"
			FROM "Module" m
			JOIN "Section" s ON m."sectionId" = s.id
			WHERE s."courseId" = $1
			ORDER BY COALESCE(m."order", 0) ASC
		`

		modulesRows, err := r.db.QueryContext(ctx, modulesQuery, courseID)
		if err != nil {
			r.log.Error("Error querying modules: " + err.Error())
			return nil, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "erro ao buscar módulos",
			}
		}
		defer modulesRows.Close()

		for modulesRows.Next() {
			var module catalog.ModuleData
			var order sql.NullInt32

			err := modulesRows.Scan(&module.ID, &module.Name, &order)
			if err != nil {
				r.log.Error("Error scanning module: " + err.Error())
				continue
			}

			if order.Valid {
				orderVal := int(order.Int32)
				module.Order = &orderVal
			}

			lessonsQuery := `
				SELECT 
					l.id,
					l.name,
					l.slug,
					l.type,
					l."mediaUrl",
					l.thumbnail,
					l."order"
				FROM "Lesson" l
				WHERE l."moduleId" = $1 AND l.published = true
				ORDER BY COALESCE(l."order", 0) ASC
			`

			lessonsRows, err := r.db.QueryContext(ctx, lessonsQuery, module.ID)
			if err != nil {
				r.log.Error("Error querying lessons: " + err.Error())
				continue
			}

			for lessonsRows.Next() {
				var lesson catalog.LessonData
				var slug, lessonType, mediaURL, thumbnail sql.NullString
				var order sql.NullInt32

				err := lessonsRows.Scan(&lesson.ID, &lesson.Name, &slug, &lessonType, &mediaURL, &thumbnail, &order)
				if err != nil {
					r.log.Error("Error scanning lesson: " + err.Error())
					continue
				}

				if slug.Valid {
					lesson.Slug = &slug.String
				}
				if lessonType.Valid {
					lesson.Type = &lessonType.String
				}
				if mediaURL.Valid {
					lesson.MediaURL = &mediaURL.String
				}
				if thumbnail.Valid {
					lesson.Thumbnail = &thumbnail.String
				}
				if order.Valid {
					orderVal := int(order.Int32)
					lesson.Order = &orderVal
				}

				module.Lessons = append(module.Lessons, lesson)
			}
			lessonsRows.Close()

			course.Modules = append(course.Modules, module)
		}
	} else {
		course.Modules = []catalog.ModuleData{}
	}

	return &catalog.CourseDetailResponse{
		Course: course,
	}, nil
}

func (r *CatalogRepository) GetModuleByID(ctx context.Context, moduleID, tenantID string, includeChildren bool) (*catalog.ModuleDetailResponse, error) {
	query := `
		SELECT 
			m.id,
			m.name,
			m."order"
		FROM "Module" m
		JOIN "Section" s ON m."sectionId" = s.id
		JOIN "Course" c ON s."courseId" = c.id
		JOIN "Vitrine" v ON c."vitrineId" = v.id
		WHERE m.id = $1 AND v."tenantId" = $2
	`

	var module catalog.ModuleData
	var order sql.NullInt32

	err := r.db.QueryRowContext(ctx, query, moduleID, tenantID).Scan(&module.ID, &module.Name, &order)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "Módulo não encontrado",
			}
		}
		r.log.Error("Error querying module: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "erro ao buscar módulo",
		}
	}

	if order.Valid {
		orderVal := int(order.Int32)
		module.Order = &orderVal
	}

	if includeChildren {
		lessonsQuery := `
			SELECT 
				l.id,
				l.name,
				l.slug,
				l.type,
				l."mediaUrl",
				l.thumbnail,
				l."order"
			FROM "Lesson" l
			WHERE l."moduleId" = $1 AND l.published = true
			ORDER BY COALESCE(l."order", 0) ASC
		`

		lessonsRows, err := r.db.QueryContext(ctx, lessonsQuery, moduleID)
		if err != nil {
			r.log.Error("Error querying lessons: " + err.Error())
			return nil, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "erro ao buscar aulas",
			}
		}
		defer lessonsRows.Close()

		for lessonsRows.Next() {
			var lesson catalog.LessonData
			var slug, lessonType, mediaURL, thumbnail sql.NullString
			var order sql.NullInt32

			err := lessonsRows.Scan(&lesson.ID, &lesson.Name, &slug, &lessonType, &mediaURL, &thumbnail, &order)
			if err != nil {
				r.log.Error("Error scanning lesson: " + err.Error())
				continue
			}

			if slug.Valid {
				lesson.Slug = &slug.String
			}
			if lessonType.Valid {
				lesson.Type = &lessonType.String
			}
			if mediaURL.Valid {
				lesson.MediaURL = &mediaURL.String
			}
			if thumbnail.Valid {
				lesson.Thumbnail = &thumbnail.String
			}
			if order.Valid {
				orderVal := int(order.Int32)
				lesson.Order = &orderVal
			}

			module.Lessons = append(module.Lessons, lesson)
		}
	} else {
		module.Lessons = []catalog.LessonData{}
	}

	return &catalog.ModuleDetailResponse{
		Module: module,
	}, nil
}

func (r *CatalogRepository) GetLessonByID(ctx context.Context, lessonID, tenantID string) (*catalog.LessonDetailResponse, error) {
	query := `
		SELECT 
			l.id,
			l.name,
			l.slug,
			l.type,
			l."mediaUrl",
			l.thumbnail,
			l."order"
		FROM "Lesson" l
		JOIN "Module" m ON l."moduleId" = m.id
		JOIN "Section" s ON m."sectionId" = s.id
		JOIN "Course" c ON s."courseId" = c.id
		JOIN "Vitrine" v ON c."vitrineId" = v.id
		WHERE l.id = $1 AND v."tenantId" = $2
	`

	var lesson catalog.LessonData
	var slug, lessonType, mediaURL, thumbnail sql.NullString
	var order sql.NullInt32

	err := r.db.QueryRowContext(ctx, query, lessonID, tenantID).Scan(&lesson.ID, &lesson.Name, &slug, &lessonType, &mediaURL, &thumbnail, &order)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &memberclasserrors.MemberClassError{
				Code:    404,
				Message: "Aula não encontrada",
			}
		}
		r.log.Error("Error querying lesson: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "erro ao buscar aula",
		}
	}

	if slug.Valid {
		lesson.Slug = &slug.String
	}
	if lessonType.Valid {
		lesson.Type = &lessonType.String
	}
	if mediaURL.Valid {
		lesson.MediaURL = &mediaURL.String
	}
	if thumbnail.Valid {
		lesson.Thumbnail = &thumbnail.String
	}
	if order.Valid {
		orderVal := int(order.Int32)
		lesson.Order = &orderVal
	}

	return &catalog.LessonDetailResponse{
		Lesson: lesson,
	}, nil
}
