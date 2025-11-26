package ports

import "github.com/memberclass-backend-golang/internal/domain/dto"

type PdfProcessService interface {
	GetToken() (string, error)
	CreateTask(token string) (*dto.TaskResponse, error)
	AddFile(token, taskID, pdfURL, server string) (string, error)
	ProcessTask(token, taskID, serverFilename, server string) error
	DownloadTask(token, taskID, server string) ([]byte, error)
	ExtractImagesFromZip(zipData []byte) ([]string, error)
}
