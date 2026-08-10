package database

import "testing"

// TestBucketDSNMapping_Transcription guards the wiring contract between the
// transcription slice and the multi-DB layer: the slice does
// `dbMap["transcription"]` to reach the Supabase pgvector connection.
// If this test breaks, the slice loses its DB at startup.
func TestBucketDSNMapping_Transcription(t *testing.T) {
	envVar, ok := bucketDSNMapping["transcription"]
	if !ok {
		t.Fatal("expected bucket 'transcription' in bucketDSNMapping")
	}
	if envVar != "DB_TRANSCRIPTION_DSN" {
		t.Fatalf("expected env DB_TRANSCRIPTION_DSN, got %s", envVar)
	}
}

func TestBucketDSNMapping_LegacyBucketsStillPresent(t *testing.T) {
	for _, bucket := range []string{"memberclass", "ephra", "celetusclass"} {
		if _, ok := bucketDSNMapping[bucket]; !ok {
			t.Fatalf("regression: bucket %q removed from bucketDSNMapping", bucket)
		}
	}
}
