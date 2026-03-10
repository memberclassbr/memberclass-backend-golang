# Design: Bucket Dinamico para Download/Upload de PDFs

**Data:** 2026-03-10
**Status:** Aprovado

## Contexto

O sistema processa PDFs de lessons convertendo-os em imagens JPEG via iLovePDF e armazenando no DigitalOcean Spaces. Atualmente, o bucket e configurado uma unica vez via `DO_SPACES_BUCKET` env var. Com multiplos buckets enviando PDFs para processar, o bucket precisa ser dinamico — extraido da URL do `lesson.MediaURL`.

## Requisitos

- `lesson.MediaURL` agora pode apontar para diferentes buckets DO Spaces
- Download do PDF: iLovePDF baixa direto pela URL (nao muda)
- Upload das imagens processadas: deve ir para o **mesmo bucket** de onde veio o PDF
- Todas os buckets usam as mesmas credenciais (`DO_SPACES_ID` / `DO_SPACES_SECRET`)
- Todos os buckets estao na mesma regiao
- `DO_SPACES_BUCKET` continua como fallback/default

## Abordagem Escolhida: Extrair bucket da URL no momento da operacao

### Camada Storage

**Interface `ports.Storage`** — adicionar `UploadToBucket`:

```go
type Storage interface {
    Upload(ctx context.Context, data []byte, filename string, contentType string) (string, error)
    UploadToBucket(ctx context.Context, bucket string, data []byte, filename string, contentType string) (string, error)
    Download(ctx context.Context, urlOrKey string) ([]byte, error)
    Delete(ctx context.Context, urlOrKey string) error
    Exists(ctx context.Context, urlOrKey string) (bool, error)
}
```

**Implementacao `DigitalOceanSpaces`:**

- `UploadToBucket`: igual ao `Upload` mas usa o bucket recebido e constroi `publicURL` dinamicamente
- `Download`, `Delete`, `Exists`: extraem bucket da URL quando recebem URL completa; usam `d.bucket` como fallback quando recebem apenas key
- `Upload` original: continua usando `d.bucket` (backward compatibility)
- Novo helper privado: `extractBucketFromURL(url) string`

**Logica de extracao de bucket:**

```
URL: https://meu-bucket.nyc3.digitaloceanspaces.com/path/file.pdf
Hostname: meu-bucket.nyc3.digitaloceanspaces.com
Bucket: meu-bucket (primeiro segmento antes do ".")
Fallback: DO_SPACES_BUCKET
```

### Camada Use Case

Propagar o bucket extraido do `lesson.MediaURL` por todo o pipeline:

```
ProcessLesson(ctx, lessonID)
  bucket := extractBucketFromMediaURL(lesson.MediaURL)
  ConvertPdfToImages(lesson.MediaURL)                          // iLovePDF, nao muda
  SavePagesDirectly(ctx, asset.ID, lessonID, images, bucket)   // recebe bucket
    saveSinglePage(ctx, assetID, pageNumber, imageBase64, bucket)
      storageService.UploadToBucket(ctx, bucket, data, filename, contentType)
```

**Metodos afetados:**

| Metodo | Mudanca |
|--------|---------|
| `ProcessLesson` | Extrai bucket do `lesson.MediaURL`, passa para `SavePagesDirectly` |
| `SavePagesDirectly` | Recebe `bucket string`, passa para `saveSinglePage` |
| `saveSinglePage` | Recebe `bucket string`, usa `UploadToBucket` ao inves de `Upload` |

### O que NAO muda

- `Upload` original (backward compatibility para outros callers)
- `ConvertPdfToImages` (iLovePDF baixa pela URL)
- Cleanup/Regenerate (operam no DB, nao no storage)
- `DO_SPACES_BUCKET` continua existindo como default

## Arquivos Afetados

| Arquivo | Tipo |
|---------|------|
| `internal/domain/ports/storage.go` | Modificar |
| `internal/infrastructure/adapters/storage/digital_ocean_spaces.go` | Modificar |
| `internal/domain/usecases/lessons/pdf_processor_usecase.go` | Modificar |

## Riscos

- **Parsing de URL incorreto:** mitigado com fallback para `DO_SPACES_BUCKET`
- **URLs nao-DO-Spaces em `lesson.MediaURL`:** fallback para bucket default
