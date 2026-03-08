package upload

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const uploadThingVersionHeaderValue = "7.7.4"

// UploadThingUploadRequest contains validated media payload for UploadThing upload.
type UploadThingUploadRequest struct {
	FilePath    string
	FileName    string
	ContentType string
	ByteSize    int64
	UploadKind  string
}

// UploadThingUploadResponse contains persisted UploadThing identity and public URL.
type UploadThingUploadResponse struct {
	FileKey  string
	MediaURL string
}

// UploadThingClient uploads one validated media payload.
type UploadThingClient interface {
	UploadFile(ctx context.Context, req UploadThingUploadRequest) (UploadThingUploadResponse, error)
}

// UploadThingMediaStorage stores validated media through UploadThing.
type UploadThingMediaStorage struct {
	client UploadThingClient
}

// NewUploadThingMediaStorage creates UploadThing-backed media storage.
func NewUploadThingMediaStorage(client UploadThingClient) (*UploadThingMediaStorage, error) {
	if client == nil {
		return nil, fmt.Errorf("uploadthing client is required")
	}

	return &UploadThingMediaStorage{client: client}, nil
}

// StoreValidated uploads validated media to UploadThing and returns a persisted media URL.
func (s *UploadThingMediaStorage) StoreValidated(
	ctx context.Context,
	media ValidatedMedia,
) (StoredMedia, func() error, error) {
	if err := ctx.Err(); err != nil {
		return StoredMedia{}, nil, err
	}
	if media.TempFilePath == "" {
		return StoredMedia{}, nil, fmt.Errorf("validated media temp file path is required")
	}
	defer media.Cleanup()

	response, err := s.client.UploadFile(ctx, UploadThingUploadRequest{
		FilePath:    media.TempFilePath,
		FileName:    sanitizeUploadFilename(media.OriginalFilename, media.Kind),
		ContentType: media.ContentType,
		ByteSize:    media.ByteSize,
		UploadKind:  media.Kind,
	})
	if err != nil {
		return StoredMedia{}, nil, err
	}

	mediaURL := strings.TrimSpace(response.MediaURL)
	if mediaURL == "" {
		return StoredMedia{}, nil, fmt.Errorf("uploadthing upload response did not include media URL")
	}
	if _, err := mustParseHTTPURL(mediaURL, "uploadthing media URL"); err != nil {
		return StoredMedia{}, nil, err
	}

	return StoredMedia{
		MediaURL: mediaURL,
	}, func() error { return nil }, nil
}

// UploadThingHTTPClient uploads files via UploadThing's prepare-upload API.
type UploadThingHTTPClient struct {
	token            string
	prepareUploadURL string
	httpClient       *http.Client
}

// NewUploadThingHTTPClient creates an UploadThing HTTP client.
func NewUploadThingHTTPClient(token string, prepareUploadURL string, timeout time.Duration) (*UploadThingHTTPClient, error) {
	normalizedToken := strings.TrimSpace(token)
	if normalizedToken == "" {
		return nil, fmt.Errorf("uploadthing token is required")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("uploadthing request timeout must be greater than zero")
	}
	normalizedURL, err := mustParseHTTPURL(prepareUploadURL, "uploadthing prepare-upload URL")
	if err != nil {
		return nil, err
	}

	return &UploadThingHTTPClient{
		token:            normalizedToken,
		prepareUploadURL: normalizedURL.String(),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (c *UploadThingHTTPClient) UploadFile(ctx context.Context, req UploadThingUploadRequest) (UploadThingUploadResponse, error) {
	if err := ctx.Err(); err != nil {
		return UploadThingUploadResponse{}, err
	}
	if strings.TrimSpace(req.FilePath) == "" {
		return UploadThingUploadResponse{}, fmt.Errorf("uploadthing upload file path is required")
	}
	if req.ByteSize <= 0 {
		return UploadThingUploadResponse{}, fmt.Errorf("uploadthing upload byte size must be greater than zero")
	}
	if strings.TrimSpace(req.ContentType) == "" {
		return UploadThingUploadResponse{}, fmt.Errorf("uploadthing upload content type is required")
	}

	fileName := sanitizeUploadFilename(req.FileName, req.UploadKind)
	fileContent, err := os.ReadFile(req.FilePath)
	if err != nil {
		return UploadThingUploadResponse{}, fmt.Errorf("read media file for uploadthing upload: %w", err)
	}

	prepareResponse, err := c.prepareUpload(ctx, fileName, req.ContentType, req.ByteSize)
	if err != nil {
		return UploadThingUploadResponse{}, err
	}

	if err := c.uploadPreparedFile(ctx, prepareResponse, fileName, req.ContentType, fileContent); err != nil {
		return UploadThingUploadResponse{}, err
	}

	mediaURL := resolveUploadThingMediaURL(prepareResponse)
	if strings.TrimSpace(mediaURL) == "" {
		return UploadThingUploadResponse{}, fmt.Errorf("uploadthing prepare-upload response did not include a media URL")
	}

	return UploadThingUploadResponse{
		FileKey:  strings.TrimSpace(prepareResponse.Key),
		MediaURL: mediaURL,
	}, nil
}

type uploadThingPrepareUploadRequest struct {
	FileName string `json:"fileName"`
	FileSize int64  `json:"fileSize"`
	FileType string `json:"fileType"`
}

type uploadThingPrepareUploadItemResponse struct {
	URL     string `json:"url"`
	Key     string `json:"key"`
	FileURL string `json:"fileUrl"`
	UFSURL  string `json:"ufsUrl"`
	AppURL  string `json:"appUrl"`
}

type uploadThingPrepareUploadEnvelope struct {
	Data []uploadThingPrepareUploadItemResponse `json:"data"`
}

type uploadThingPrepareUploadEnvelopeSingle struct {
	Data uploadThingPrepareUploadItemResponse `json:"data"`
}

func (c *UploadThingHTTPClient) prepareUpload(
	ctx context.Context,
	fileName string,
	contentType string,
	byteSize int64,
) (uploadThingPrepareUploadItemResponse, error) {
	payload := uploadThingPrepareUploadRequest{
		FileName: fileName,
		FileSize: byteSize,
		FileType: contentType,
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return uploadThingPrepareUploadItemResponse{}, fmt.Errorf("marshal uploadthing prepare-upload payload: %w", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.prepareUploadURL,
		bytes.NewReader(rawPayload),
	)
	if err != nil {
		return uploadThingPrepareUploadItemResponse{}, fmt.Errorf("build uploadthing prepare-upload request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("x-uploadthing-api-key", c.token)
	request.Header.Set("x-uploadthing-version", uploadThingVersionHeaderValue)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return uploadThingPrepareUploadItemResponse{}, fmt.Errorf("request uploadthing prepare-upload: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return uploadThingPrepareUploadItemResponse{}, fmt.Errorf("read uploadthing prepare-upload response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return uploadThingPrepareUploadItemResponse{}, fmt.Errorf(
			"uploadthing prepare-upload failed with status %d: %s",
			response.StatusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}

	item, err := decodeUploadThingPrepareUploadResponse(responseBody)
	if err != nil {
		return uploadThingPrepareUploadItemResponse{}, err
	}
	if strings.TrimSpace(item.URL) == "" {
		return uploadThingPrepareUploadItemResponse{}, fmt.Errorf("uploadthing prepare-upload response did not include upload URL")
	}

	return item, nil
}

func decodeUploadThingPrepareUploadResponse(raw []byte) (uploadThingPrepareUploadItemResponse, error) {
	var single uploadThingPrepareUploadItemResponse
	if err := json.Unmarshal(raw, &single); err == nil {
		if strings.TrimSpace(single.URL) != "" || strings.TrimSpace(single.Key) != "" {
			return single, nil
		}
	}

	var direct []uploadThingPrepareUploadItemResponse
	if err := json.Unmarshal(raw, &direct); err == nil {
		if len(direct) > 0 {
			return direct[0], nil
		}
	}

	var envelope uploadThingPrepareUploadEnvelope
	if err := json.Unmarshal(raw, &envelope); err == nil {
		if len(envelope.Data) > 0 {
			return envelope.Data[0], nil
		}
	}

	var envelopeSingle uploadThingPrepareUploadEnvelopeSingle
	if err := json.Unmarshal(raw, &envelopeSingle); err == nil {
		if strings.TrimSpace(envelopeSingle.Data.URL) != "" || strings.TrimSpace(envelopeSingle.Data.Key) != "" {
			return envelopeSingle.Data, nil
		}
	}

	return uploadThingPrepareUploadItemResponse{}, fmt.Errorf("decode uploadthing prepare-upload response")
}

func (c *UploadThingHTTPClient) uploadPreparedFile(
	ctx context.Context,
	item uploadThingPrepareUploadItemResponse,
	fileName string,
	contentType string,
	fileContent []byte,
) error {
	uploadURL := strings.TrimSpace(item.URL)
	if uploadURL == "" {
		return fmt.Errorf("uploadthing upload URL is required")
	}

	parsedUploadURL, err := mustParseHTTPURL(uploadURL, "uploadthing upload URL")
	if err != nil {
		return err
	}

	isPutObjectUpload := strings.EqualFold(parsedUploadURL.Query().Get("x-id"), "PutObject")
	httpMethod := http.MethodPut

	var request *http.Request
	if isPutObjectUpload {
		request, err = http.NewRequestWithContext(
			ctx,
			httpMethod,
			parsedUploadURL.String(),
			bytes.NewReader(fileContent),
		)
		if err != nil {
			return fmt.Errorf("build uploadthing upload request: %w", err)
		}
		request.Header.Set("Content-Type", contentType)
	} else {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", fileName)
		if err != nil {
			return fmt.Errorf("create uploadthing multipart file part: %w", err)
		}
		if _, err := io.Copy(part, bytes.NewReader(fileContent)); err != nil {
			return fmt.Errorf("write uploadthing multipart payload: %w", err)
		}
		if err := writer.Close(); err != nil {
			return fmt.Errorf("close uploadthing multipart payload: %w", err)
		}

		request, err = http.NewRequestWithContext(
			ctx,
			httpMethod,
			parsedUploadURL.String(),
			bytes.NewReader(body.Bytes()),
		)
		if err != nil {
			return fmt.Errorf("build uploadthing upload request: %w", err)
		}
		request.Header.Set("Content-Type", writer.FormDataContentType())
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("request uploadthing upload URL: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}

	responseBody, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		return fmt.Errorf("uploadthing upload URL failed with status %d", response.StatusCode)
	}
	return fmt.Errorf(
		"uploadthing upload URL failed with status %d: %s",
		response.StatusCode,
		strings.TrimSpace(string(responseBody)),
	)
}

func resolveUploadThingMediaURL(item uploadThingPrepareUploadItemResponse) string {
	candidates := []string{
		item.FileURL,
		item.UFSURL,
		item.AppURL,
	}
	for _, candidate := range candidates {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "" {
			continue
		}
		if _, err := mustParseHTTPURL(trimmed, "uploadthing media URL"); err == nil {
			return trimmed
		}
	}

	key := strings.TrimSpace(item.Key)
	if key == "" {
		return ""
	}
	return "https://utfs.io/f/" + key
}

func sanitizeUploadFilename(fileName string, uploadKind string) string {
	normalized := strings.TrimSpace(filepath.Base(fileName))
	if normalized == "" || normalized == "." || normalized == string(filepath.Separator) {
		switch strings.ToLower(strings.TrimSpace(uploadKind)) {
		case KindImage:
			return "upload-image.bin"
		case KindVideo:
			return "upload-video.bin"
		default:
			return "upload-file.bin"
		}
	}

	return normalized
}

func mustParseHTTPURL(raw string, fieldName string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", fieldName, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%s must use http or https scheme", fieldName)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return nil, fmt.Errorf("%s must include host", fieldName)
	}
	return parsed, nil
}
