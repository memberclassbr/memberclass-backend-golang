package student_report

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
	studentreq "github.com/memberclass-backend-golang/internal/domain/dto/request/student"
	"github.com/memberclass-backend-golang/internal/domain/dto/response/student"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/domain/ports"
	student2 "github.com/memberclass-backend-golang/internal/domain/ports/student"
)

type StudentReportRepository struct {
	db  *sql.DB
	log ports.Logger
}

func NewStudentReportRepository(db *sql.DB, log ports.Logger) student2.StudentReportRepository {
	return &StudentReportRepository{
		db:  db,
		log: log,
	}
}

const maxRankingLimit = 50000

func (r *StudentReportRepository) GetStudentsRanking(ctx context.Context, req studentreq.GetStudentsRankingRequest, start, end time.Time) ([]student.StudentRankingRow, int64, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = maxRankingLimit
	}
	offset := (req.Page - 1) * limit
	orderBy := r.rankingOrderBy(req.Metric)

	query := fmt.Sprintf(`
		WITH members AS (
			SELECT uot."userId"
			FROM "UsersOnTenants" uot
			WHERE uot."tenantId" = $1 AND uot.role = 'member'
		),
		logins_cte AS (
			SELECT ue."usersOnTenantsUserId" AS user_id, COUNT(*) AS cnt
			FROM "UserEvent" ue
			WHERE ue.type = 'login' AND ue."usersOnTenantsTenantId" = $1 AND ue."createdAt" BETWEEN $2 AND $3
			GROUP BY ue."usersOnTenantsUserId"
		),
		lessons_cte AS (
			SELECT r."userId" AS user_id, COUNT(*) AS cnt
			FROM "Read" r
			JOIN "Lesson" l ON r."lessonId" = l.id
			JOIN "Module" m ON l."moduleId" = m.id
			JOIN "Section" s ON m."sectionId" = s.id
			JOIN "Course" c ON s."courseId" = c.id
			JOIN "Vitrine" v ON c."vitrineId" = v.id
			WHERE v."tenantId" = $1 AND r."createdAt" BETWEEN $2 AND $3
			GROUP BY r."userId"
		),
		comments_cte AS (
			SELECT c."userId" AS user_id, COUNT(*) AS cnt
			FROM "Comment" c
			JOIN "Lesson" l ON c."lessonId" = l.id
			JOIN "Module" m ON l."moduleId" = m.id
			JOIN "Section" s ON m."sectionId" = s.id
			JOIN "Course" co ON s."courseId" = co.id
			JOIN "Vitrine" v ON co."vitrineId" = v.id
			WHERE v."tenantId" = $1 AND c."createdAt" BETWEEN $2 AND $3
			GROUP BY c."userId"
		),
		ratings_cte AS (
			SELECT r."userId" AS user_id, COUNT(*) AS cnt, AVG(r.rating) AS avg_rating
			FROM "Read" r
			JOIN "Lesson" l ON r."lessonId" = l.id
			JOIN "Module" m ON l."moduleId" = m.id
			JOIN "Section" s ON m."sectionId" = s.id
			JOIN "Course" co ON s."courseId" = co.id
			JOIN "Vitrine" v ON co."vitrineId" = v.id
			WHERE v."tenantId" = $1 AND r.rating IS NOT NULL AND r."createdAt" BETWEEN $2 AND $3
			GROUP BY r."userId"
		)
		SELECT u.id, u.email, COALESCE(uot.name, u.username, ''), u.image,
			COALESCE(l.cnt, 0)::int, COALESCE(less.cnt, 0)::int, COALESCE(c.cnt, 0)::int, COALESCE(rat.cnt, 0)::int, COALESCE(rat.avg_rating, 0)::float8
		FROM members m
		JOIN "User" u ON u.id = m."userId"
		JOIN "UsersOnTenants" uot ON uot."userId" = m."userId" AND uot."tenantId" = $1
		LEFT JOIN logins_cte l ON l.user_id = m."userId"
		LEFT JOIN lessons_cte less ON less.user_id = m."userId"
		LEFT JOIN comments_cte c ON c.user_id = m."userId"
		LEFT JOIN ratings_cte rat ON rat.user_id = m."userId"
		%s
		LIMIT $4 OFFSET $5
	`, orderBy)

	args := []interface{}{req.TenantID, start, end, limit, offset}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		r.log.Error("Error getting students ranking: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting students ranking",
		}
	}
	defer rows.Close()

	var result []student.StudentRankingRow
	for rows.Next() {
		var row student.StudentRankingRow
		var name string
		var image sql.NullString
		if err := rows.Scan(&row.UserID, &row.Email, &name, &image, &row.Logins, &row.LessonsWatched, &row.Comments, &row.Ratings, &row.AvgRating); err != nil {
			r.log.Error("Error scanning ranking row: " + err.Error())
			return nil, 0, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error scanning ranking",
			}
		}
		row.Name = name
		if image.Valid {
			row.Picture = &image.String
		}
		result = append(result, row)
	}

	if err = rows.Err(); err != nil {
		r.log.Error("Error iterating ranking: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error iterating ranking",
		}
	}

	countQuery := `
		SELECT COUNT(*) FROM "UsersOnTenants" WHERE "tenantId" = $1 AND role = 'member'
	`
	var total int64
	err = r.db.QueryRowContext(ctx, countQuery, req.TenantID).Scan(&total)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return result, 0, nil
		}
		r.log.Error("Error counting members: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error counting members",
		}
	}

	return result, total, nil
}

func (r *StudentReportRepository) rankingOrderBy(metric string) string {
	switch metric {
	case studentreq.MetricLogins:
		return `ORDER BY COALESCE(l.cnt, 0) DESC`
	case studentreq.MetricLessons:
		return `ORDER BY COALESCE(less.cnt, 0) DESC`
	case studentreq.MetricComments:
		return `ORDER BY COALESCE(c.cnt, 0) DESC`
	case studentreq.MetricRatings:
		return `ORDER BY COALESCE(rat.cnt, 0) DESC`
	default:
		return `ORDER BY (COALESCE(l.cnt, 0) + COALESCE(less.cnt, 0) + COALESCE(c.cnt, 0) + COALESCE(rat.cnt, 0)) DESC`
	}
}

func (r *StudentReportRepository) GetStudentsReport(ctx context.Context, tenantID string, startDate, endDate *time.Time, page, limit int) ([]student.StudentReport, int64, error) {
	offset := (page - 1) * limit

	query := `
		SELECT 
			uot."userId",
			u.email,
			COALESCE(uot.document, '') as cpf,
			uot."assignedAt"
		FROM "UsersOnTenants" uot
		JOIN "User" u ON uot."userId" = u.id
		WHERE uot."tenantId" = $1
	`

	args := []interface{}{tenantID}
	argIndex := 2

	if startDate != nil {
		query += fmt.Sprintf(` AND uot."assignedAt" >= $%d`, argIndex)
		args = append(args, *startDate)
		argIndex++
	}

	if endDate != nil {
		query += fmt.Sprintf(` AND uot."assignedAt" <= $%d`, argIndex)
		args = append(args, *endDate)
		argIndex++
	}

	query += fmt.Sprintf(` ORDER BY uot."assignedAt" DESC LIMIT $%d OFFSET $%d`, argIndex, argIndex+1)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		r.log.Error("Error getting students report: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting students report",
		}
	}
	defer rows.Close()

	studentsMap := make(map[string]*student.StudentReport)

	for rows.Next() {
		var userID, email, cpf string
		var assignedAt time.Time

		if err := rows.Scan(&userID, &email, &cpf, &assignedAt); err != nil {
			r.log.Error("Error scanning student: " + err.Error())
			return nil, 0, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error scanning student",
			}
		}

		if _, exists := studentsMap[userID]; !exists {
			studentsMap[userID] = &student.StudentReport{
				AlunoIDMemberClass:        userID,
				Email:                     email,
				Cpf:                       cpf,
				DataCadastro:              assignedAt.Format(time.RFC3339),
				EntregasVinculadas:        []string{},
				UltimoAcesso:              nil,
				QuantidadeAulasAssistidas: 0,
				AulasAssistidas:           []student.LessonWatched{},
			}
		}
	}

	if err = rows.Err(); err != nil {
		r.log.Error("Error iterating students: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error iterating students",
		}
	}

	var userIDs []string
	for userID := range studentsMap {
		userIDs = append(userIDs, userID)
	}

	if len(userIDs) == 0 {
		return []student.StudentReport{}, 0, nil
	}

	deliveries, err := r.getDeliveries(ctx, tenantID)
	if err != nil {
		return nil, 0, err
	}

	userOnDeliveries, memberOnDeliveries, err := r.getUserDeliveries(ctx, userIDs, tenantID)
	if err != nil {
		return nil, 0, err
	}

	lessonsWatched, err := r.getLessonsWatched(ctx, userIDs, tenantID)
	if err != nil {
		return nil, 0, err
	}

	lastAccesses, err := r.getLastAccesses(ctx, userIDs, tenantID)
	if err != nil {
		return nil, 0, err
	}

	for userID, student := range studentsMap {
		deliveryIDs := []string{}
		if userDeliveries, ok := userOnDeliveries[userID]; ok {
			deliveryIDs = append(deliveryIDs, userDeliveries...)
		}
		if memberDeliveries, ok := memberOnDeliveries[userID]; ok {
			deliveryIDs = append(deliveryIDs, memberDeliveries...)
		}

		deliveryNames := []string{}
		for _, deliveryID := range deliveryIDs {
			if name, ok := deliveries[deliveryID]; ok {
				if !contains(deliveryNames, name) {
					deliveryNames = append(deliveryNames, name)
				}
			}
		}

		student.EntregasVinculadas = deliveryNames

		if lessons, ok := lessonsWatched[userID]; ok {
			student.AulasAssistidas = lessons
			student.QuantidadeAulasAssistidas = len(lessons)
		}

		if lastAccess, ok := lastAccesses[userID]; ok {
			formatted := lastAccess.Format(time.RFC3339)
			student.UltimoAcesso = &formatted
		}
	}

	students := make([]student.StudentReport, 0, len(studentsMap))
	for _, student := range studentsMap {
		students = append(students, *student)
	}

	countQuery := `SELECT COUNT(*) FROM "UsersOnTenants" WHERE "tenantId" = $1`
	countArgs := []interface{}{tenantID}
	countArgIndex := 2

	if startDate != nil {
		countQuery += fmt.Sprintf(` AND "assignedAt" >= $%d`, countArgIndex)
		countArgs = append(countArgs, *startDate)
		countArgIndex++
	}

	if endDate != nil {
		countQuery += fmt.Sprintf(` AND "assignedAt" <= $%d`, countArgIndex)
		countArgs = append(countArgs, *endDate)
	}

	var totalCount int64
	err = r.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&totalCount)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return students, 0, nil
		}
		r.log.Error("Error counting students: " + err.Error())
		return nil, 0, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error counting students",
		}
	}

	return students, totalCount, nil
}

func (r *StudentReportRepository) getDeliveries(ctx context.Context, tenantID string) (map[string]string, error) {
	query := `SELECT id, name FROM "Delivery" WHERE "tenantId" = $1`

	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		r.log.Error("Error getting deliveries: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting deliveries",
		}
	}
	defer rows.Close()

	deliveries := make(map[string]string)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			r.log.Error("Error scanning delivery: " + err.Error())
			return nil, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error scanning delivery",
			}
		}
		deliveries[id] = name
	}

	return deliveries, nil
}

func (r *StudentReportRepository) getUserDeliveries(ctx context.Context, userIDs []string, tenantID string) (map[string][]string, map[string][]string, error) {
	userOnDeliveryQuery := `SELECT "userId", "deliveryId" FROM "UserOnDelivery" WHERE "userId" = ANY($1)`

	rows, err := r.db.QueryContext(ctx, userOnDeliveryQuery, pq.Array(userIDs))
	if err != nil {
		r.log.Error("Error getting user deliveries: " + err.Error())
		return nil, nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting user deliveries",
		}
	}
	defer rows.Close()

	userOnDeliveries := make(map[string][]string)
	for rows.Next() {
		var userID, deliveryID string
		if err := rows.Scan(&userID, &deliveryID); err != nil {
			r.log.Error("Error scanning user delivery: " + err.Error())
			return nil, nil, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error scanning user delivery",
			}
		}
		userOnDeliveries[userID] = append(userOnDeliveries[userID], deliveryID)
	}

	memberOnDeliveryQuery := `SELECT "memberId", "deliveryId" FROM "MemberOnDelivery" WHERE "memberId" = ANY($1) AND "tenantId" = $2`

	memberRows, err := r.db.QueryContext(ctx, memberOnDeliveryQuery, pq.Array(userIDs), tenantID)
	if err != nil {
		r.log.Error("Error getting member deliveries: " + err.Error())
		return nil, nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting member deliveries",
		}
	}
	defer memberRows.Close()

	memberOnDeliveries := make(map[string][]string)
	for memberRows.Next() {
		var memberID, deliveryID string
		if err := memberRows.Scan(&memberID, &deliveryID); err != nil {
			r.log.Error("Error scanning member delivery: " + err.Error())
			return nil, nil, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error scanning member delivery",
			}
		}
		memberOnDeliveries[memberID] = append(memberOnDeliveries[memberID], deliveryID)
	}

	return userOnDeliveries, memberOnDeliveries, nil
}

func (r *StudentReportRepository) getLessonsWatched(ctx context.Context, userIDs []string, tenantID string) (map[string][]student.LessonWatched, error) {
	query := `
		SELECT 
			r."userId",
			r."lessonId",
			l.name as lesson_name,
			r."createdAt"
		FROM "Read" r
		JOIN "Lesson" l ON r."lessonId" = l.id
		JOIN "Module" m ON l."moduleId" = m.id
		JOIN "Section" s ON m."sectionId" = s.id
		JOIN "Course" c ON s."courseId" = c.id
		JOIN "Vitrine" v ON c."vitrineId" = v.id
		WHERE r."userId" = ANY($1)
		  AND v."tenantId" = $2
		ORDER BY r."createdAt" DESC
	`

	rows, err := r.db.QueryContext(ctx, query, pq.Array(userIDs), tenantID)
	if err != nil {
		r.log.Error("Error getting lessons watched: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting lessons watched",
		}
	}
	defer rows.Close()

	lessonsMap := make(map[string][]student.LessonWatched)
	for rows.Next() {
		var userID, lessonID, lessonName string
		var createdAt time.Time

		if err := rows.Scan(&userID, &lessonID, &lessonName, &createdAt); err != nil {
			r.log.Error("Error scanning lesson: " + err.Error())
			return nil, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error scanning lesson",
			}
		}

		lessonsMap[userID] = append(lessonsMap[userID], student.LessonWatched{
			AulaID:        lessonID,
			Titulo:        lessonName,
			DataAssistida: createdAt.Format(time.RFC3339),
		})
	}

	return lessonsMap, nil
}

func (r *StudentReportRepository) getLastAccesses(ctx context.Context, userIDs []string, tenantID string) (map[string]time.Time, error) {
	query := `
		SELECT DISTINCT ON ("usersOnTenantsUserId") 
			"usersOnTenantsUserId", 
			"createdAt"
		FROM "UserEvent"
		WHERE "usersOnTenantsUserId" = ANY($1)
		  AND "usersOnTenantsTenantId" = $2
		  AND type = 'login'
		ORDER BY "usersOnTenantsUserId", "createdAt" DESC
	`

	rows, err := r.db.QueryContext(ctx, query, pq.Array(userIDs), tenantID)
	if err != nil {
		r.log.Error("Error getting last accesses: " + err.Error())
		return nil, &memberclasserrors.MemberClassError{
			Code:    500,
			Message: "error getting last accesses",
		}
	}
	defer rows.Close()

	lastAccesses := make(map[string]time.Time)
	for rows.Next() {
		var userID string
		var createdAt time.Time

		if err := rows.Scan(&userID, &createdAt); err != nil {
			r.log.Error("Error scanning last access: " + err.Error())
			return nil, &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "error scanning last access",
			}
		}

		if _, exists := lastAccesses[userID]; !exists {
			lastAccesses[userID] = createdAt
		}
	}

	return lastAccesses, nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
