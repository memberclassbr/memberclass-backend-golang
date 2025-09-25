package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/memberclass-backend-golang/internal/domain/dto"
	"github.com/memberclass-backend-golang/internal/domain/memberclasserrors"
	"github.com/memberclass-backend-golang/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestUploadVideoBunnyCdnUseCase_Execute(t *testing.T) {
	tests := []struct {
		name           string
		bunnyParams    dto.BunnyParametersAccess
		fileBytes      []byte
		title          string
		mockSetup      func(*mocks.MockBunnyService, *mocks.MockLogger)
		expectedError  error
		expectedResult *dto.UploadVideoResponse
	}{
		{
			name: "should upload video successfully with existing social collection",
			bunnyParams: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "test-key",
			},
			fileBytes: []byte("fake video content"),
			title:     "Test Video",
			mockSetup: func(mockBunny *mocks.MockBunnyService, mockLogger *mocks.MockLogger) {
				mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

				mockLogger.EXPECT().Debug(mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()

				collections := &dto.BunnyCollectionsResponse{
					Items: []dto.BunnyCollection{
						{GUID: "social-collection-guid", Name: "social"},
					},
				}
				mockBunny.EXPECT().GetCollections(mock.Anything, mock.Anything).Return(collections, nil)

				createVideoResp := &dto.CreateVideoResponse{GUID: "video-guid"}
				mockBunny.EXPECT().CreateVideo(mock.Anything, mock.Anything, mock.Anything).Return(createVideoResp, nil)

				mockBunny.EXPECT().UploadVideo(mock.Anything, mock.Anything, mock.Anything).Return(nil)
			},
			expectedError: nil,
			expectedResult: &dto.UploadVideoResponse{
				OK:       true,
				MediaURL: "https://iframe.mediadelivery.net/embed/test-library/video-guid?autoplay=false&loop=false&muted=false&preload=true&responsive=true",
				GUID:     "video-guid",
				Title:    "Test Video",
			},
		},
		{
			name: "should upload video successfully creating new social collection",
			bunnyParams: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "test-key",
			},
			fileBytes: []byte("fake video content"),
			title:     "Test Video",
			mockSetup: func(mockBunny *mocks.MockBunnyService, mockLogger *mocks.MockLogger) {
				mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

				mockLogger.EXPECT().Debug(mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Info(mock.Anything).Return()
				mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything).Return()

				collections := &dto.BunnyCollectionsResponse{
					Items: []dto.BunnyCollection{
						{GUID: "other-collection-guid", Name: "other"},
					},
				}
				mockBunny.EXPECT().GetCollections(mock.Anything, mock.Anything).Return(collections, nil)

				createCollectionResp := &dto.CreateCollectionResponse{GUID: "new-social-guid"}
				mockBunny.EXPECT().CreateCollection(mock.Anything, mock.Anything, mock.Anything).Return(createCollectionResp, nil)

				createVideoResp := &dto.CreateVideoResponse{GUID: "video-guid"}
				mockBunny.EXPECT().CreateVideo(mock.Anything, mock.Anything, mock.Anything).Return(createVideoResp, nil)

				mockBunny.EXPECT().UploadVideo(mock.Anything, mock.Anything, mock.Anything).Return(nil)
			},
			expectedError: nil,
			expectedResult: &dto.UploadVideoResponse{
				OK:       true,
				MediaURL: "https://iframe.mediadelivery.net/embed/test-library/video-guid?autoplay=false&loop=false&muted=false&preload=true&responsive=true",
				GUID:     "video-guid",
				Title:    "Test Video",
			},
		},
		{
			name: "should return error when CreateVideo fails",
			bunnyParams: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "test-key",
			},
			fileBytes: []byte("fake video content"),
			title:     "Test Video",
			mockSetup: func(mockBunny *mocks.MockBunnyService, mockLogger *mocks.MockLogger) {
				mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Error(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

				mockLogger.EXPECT().Debug(mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()

				collections := &dto.BunnyCollectionsResponse{
					Items: []dto.BunnyCollection{
						{GUID: "social-collection-guid", Name: "social"},
					},
				}
				mockBunny.EXPECT().GetCollections(mock.Anything, mock.Anything).Return(collections, nil)

				mockBunny.EXPECT().CreateVideo(mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("create video error"))
			},
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "Error creating video",
			},
			expectedResult: nil,
		},
		{
			name: "should return error when UploadVideo fails",
			bunnyParams: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "test-key",
			},
			fileBytes: []byte("fake video content"),
			title:     "Test Video",
			mockSetup: func(mockBunny *mocks.MockBunnyService, mockLogger *mocks.MockLogger) {
				mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Error(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

				mockLogger.EXPECT().Debug(mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()

				collections := &dto.BunnyCollectionsResponse{
					Items: []dto.BunnyCollection{
						{GUID: "social-collection-guid", Name: "social"},
					},
				}
				mockBunny.EXPECT().GetCollections(mock.Anything, mock.Anything).Return(collections, nil)

				createVideoResp := &dto.CreateVideoResponse{GUID: "video-guid"}
				mockBunny.EXPECT().CreateVideo(mock.Anything, mock.Anything, mock.Anything).Return(createVideoResp, nil)

				mockBunny.EXPECT().UploadVideo(mock.Anything, mock.Anything, mock.Anything).Return(errors.New("upload error"))
			},
			expectedError: &memberclasserrors.MemberClassError{
				Code:    500,
				Message: "Error send video",
			},
			expectedResult: nil,
		},
		{
			name: "should handle GetCollections error gracefully",
			bunnyParams: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "test-key",
			},
			fileBytes: []byte("fake video content"),
			title:     "Test Video",
			mockSetup: func(mockBunny *mocks.MockBunnyService, mockLogger *mocks.MockLogger) {
				mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

				mockLogger.EXPECT().Debug(mock.Anything).Return()
				mockLogger.EXPECT().Warn(mock.Anything, mock.Anything, mock.Anything).Return()

				mockBunny.EXPECT().GetCollections(mock.Anything, mock.Anything).Return(nil, errors.New("get collections error"))

				createVideoResp := &dto.CreateVideoResponse{GUID: "video-guid"}
				mockBunny.EXPECT().CreateVideo(mock.Anything, mock.Anything, mock.Anything).Return(createVideoResp, nil)

				mockBunny.EXPECT().UploadVideo(mock.Anything, mock.Anything, mock.Anything).Return(nil)
			},
			expectedError: nil,
			expectedResult: &dto.UploadVideoResponse{
				OK:       true,
				MediaURL: "https://iframe.mediadelivery.net/embed/test-library/video-guid?autoplay=false&loop=false&muted=false&preload=true&responsive=true",
				GUID:     "video-guid",
				Title:    "Test Video",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockBunnyService := mocks.NewMockBunnyService(t)
			mockLogger := mocks.NewMockLogger(t)

			tt.mockSetup(mockBunnyService, mockLogger)

			useCase := NewUploadVideoBunnyCdnUseCase(mockBunnyService, mockLogger)

			result, err := useCase.Execute(context.Background(), tt.bunnyParams, tt.fileBytes, tt.title)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestUploadVideoBunnyCdnUseCase_ensureSocialCollection(t *testing.T) {
	tests := []struct {
		name        string
		bunnyParams dto.BunnyParametersAccess
		mockSetup   func(*mocks.MockBunnyService, *mocks.MockLogger)
		expectedID  string
	}{
		{
			name: "should return existing social collection ID",
			bunnyParams: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "test-key",
			},
			mockSetup: func(mockBunny *mocks.MockBunnyService, mockLogger *mocks.MockLogger) {
				mockLogger.EXPECT().Debug(mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()

				collections := &dto.BunnyCollectionsResponse{
					Items: []dto.BunnyCollection{
						{GUID: "social-collection-guid", Name: "social"},
					},
				}
				mockBunny.EXPECT().GetCollections(mock.Anything, mock.Anything).Return(collections, nil)
			},
			expectedID: "social-collection-guid",
		},
		{
			name: "should create new social collection when not found",
			bunnyParams: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "test-key",
			},
			mockSetup: func(mockBunny *mocks.MockBunnyService, mockLogger *mocks.MockLogger) {
				mockLogger.EXPECT().Debug(mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Info(mock.Anything).Return()
				mockLogger.EXPECT().Info(mock.Anything, mock.Anything, mock.Anything).Return()

				collections := &dto.BunnyCollectionsResponse{
					Items: []dto.BunnyCollection{
						{GUID: "other-collection-guid", Name: "other"},
					},
				}
				mockBunny.EXPECT().GetCollections(mock.Anything, mock.Anything).Return(collections, nil)

				createCollectionResp := &dto.CreateCollectionResponse{GUID: "new-social-guid"}
				mockBunny.EXPECT().CreateCollection(mock.Anything, mock.Anything, mock.Anything).Return(createCollectionResp, nil)
			},
			expectedID: "new-social-guid",
		},
		{
			name: "should return empty string when GetCollections fails",
			bunnyParams: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "test-key",
			},
			mockSetup: func(mockBunny *mocks.MockBunnyService, mockLogger *mocks.MockLogger) {
				mockLogger.EXPECT().Debug(mock.Anything).Return()
				mockLogger.EXPECT().Warn(mock.Anything, mock.Anything, mock.Anything).Return()

				mockBunny.EXPECT().GetCollections(mock.Anything, mock.Anything).Return(nil, errors.New("get collections error"))
			},
			expectedID: "",
		},
		{
			name: "should return empty string when CreateCollection fails",
			bunnyParams: dto.BunnyParametersAccess{
				LibraryID:    "test-library",
				LibraryApiKey: "test-key",
			},
			mockSetup: func(mockBunny *mocks.MockBunnyService, mockLogger *mocks.MockLogger) {
				mockLogger.EXPECT().Debug(mock.Anything).Return()
				mockLogger.EXPECT().Debug(mock.Anything, mock.Anything, mock.Anything).Return()
				mockLogger.EXPECT().Info(mock.Anything).Return()
				mockLogger.EXPECT().Warn(mock.Anything, mock.Anything, mock.Anything).Return()

				collections := &dto.BunnyCollectionsResponse{
					Items: []dto.BunnyCollection{
						{GUID: "other-collection-guid", Name: "other"},
					},
				}
				mockBunny.EXPECT().GetCollections(mock.Anything, mock.Anything).Return(collections, nil)

				mockBunny.EXPECT().CreateCollection(mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("create collection error"))
			},
			expectedID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockBunnyService := mocks.NewMockBunnyService(t)
			mockLogger := mocks.NewMockLogger(t)

			tt.mockSetup(mockBunnyService, mockLogger)

			useCase := &UploadVideoBunnyCdnUseCase{
				bunnyService: mockBunnyService,
				log:          mockLogger,
			}

			result := useCase.ensureSocialCollection(context.Background(), tt.bunnyParams)

			assert.Equal(t, tt.expectedID, result)
		})
	}
}

func TestUploadVideoBunnyCdnUseCase_generatedMediaUrl(t *testing.T) {
	tests := []struct {
		name      string
		libraryID string
		guid      string
		expected  string
	}{
		{
			name:      "should generate correct media URL",
			libraryID: "test-library",
			guid:      "video-guid",
			expected:  "https://iframe.mediadelivery.net/embed/test-library/video-guid?autoplay=false&loop=false&muted=false&preload=true&responsive=true",
		},
		{
			name:      "should handle empty library ID",
			libraryID: "",
			guid:      "video-guid",
			expected:  "https://iframe.mediadelivery.net/embed//video-guid?autoplay=false&loop=false&muted=false&preload=true&responsive=true",
		},
		{
			name:      "should handle empty GUID",
			libraryID: "test-library",
			guid:      "",
			expected:  "https://iframe.mediadelivery.net/embed/test-library/?autoplay=false&loop=false&muted=false&preload=true&responsive=true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useCase := &UploadVideoBunnyCdnUseCase{}

			result := useCase.generatedMediaUrl(context.Background(), tt.libraryID, tt.guid)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewUploadVideoBunnyCdnUseCase(t *testing.T) {
	t.Run("should create new use case instance", func(t *testing.T) {
		mockBunnyService := mocks.NewMockBunnyService(t)
		mockLogger := mocks.NewMockLogger(t)

		useCase := NewUploadVideoBunnyCdnUseCase(mockBunnyService, mockLogger)

		assert.NotNil(t, useCase)
	})
}