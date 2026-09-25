package aws_data_export

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseManifest(t *testing.T) {
	t.Run("object entries (CUR 2.0 shape)", func(t *testing.T) {
		m, err := ParseManifest(fixture(t, "nullstone-focus-Manifest.json"))
		require.NoError(t, err)
		assert.Equal(t, "nullstone-focus", m.ExportName)
		assert.Equal(t, "FOCUS_1_2_AWS", m.TableName)
		assert.Contains(t, m.Columns, "ListCost")
		assert.Equal(t, []string{
			"exports/nullstone-focus/data/BILLING_PERIOD=2026-08/nullstone-focus-00001.csv.gz",
			"exports/nullstone-focus/data/BILLING_PERIOD=2026-08/nullstone-focus-00002.csv.gz",
		}, m.DataFiles)
	})

	t.Run("string entries", func(t *testing.T) {
		m, err := ParseManifest([]byte(`{"columns":["A","B"],"dataFiles":["x/data/BILLING_PERIOD=2026-08/x-00001.csv.gz"]}`))
		require.NoError(t, err)
		assert.Equal(t, []string{"A", "B"}, m.Columns)
		assert.Equal(t, []string{"x/data/BILLING_PERIOD=2026-08/x-00001.csv.gz"}, m.DataFiles)
	})

	t.Run("path and uri entry fields", func(t *testing.T) {
		m, err := ParseManifest([]byte(`{"dataFiles":[{"path":"a.csv.gz"},{"uri":"s3://b/c.csv.gz"}]}`))
		require.NoError(t, err)
		assert.Equal(t, []string{"a.csv.gz", "s3://b/c.csv.gz"}, m.DataFiles)
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := ParseManifest([]byte(`{`))
		require.Error(t, err)
	})
}

func TestReadManifest(t *testing.T) {
	ctx := context.Background()

	t.Run("normalizes absolute uris and records the etag", func(t *testing.T) {
		f := newFakeS3()
		f.put(testLocation.ManifestKey(aug2026), []byte(`{"dataFiles":["s3://ns-billing/exports/nullstone-focus/data/BILLING_PERIOD=2026-08/nullstone-focus-00001.csv.gz","/exports/nullstone-focus/data/BILLING_PERIOD=2026-08/nullstone-focus-00002.csv.gz"]}`))
		m, err := ReadManifest(ctx, f, testLocation, aug2026)
		require.NoError(t, err)
		assert.Equal(t, []string{
			"exports/nullstone-focus/data/BILLING_PERIOD=2026-08/nullstone-focus-00001.csv.gz",
			"exports/nullstone-focus/data/BILLING_PERIOD=2026-08/nullstone-focus-00002.csv.gz",
		}, m.DataFiles)
		assert.NotEmpty(t, m.ETag)
	})

	t.Run("falls back to listing the partition when no files are listed", func(t *testing.T) {
		f := newFakeS3()
		f.put(testLocation.ManifestKey(aug2026), []byte(`{"exportName":"nullstone-focus"}`))
		f.put(testLocation.DataPrefix(aug2026)+"nullstone-focus-00002.csv.gz", []byte{})
		f.put(testLocation.DataPrefix(aug2026)+"nullstone-focus-00001.csv.gz", []byte{})
		f.put(testLocation.DataPrefix(aug2026)+"nullstone-focus-RedshiftCommands.sql", []byte{})
		m, err := ReadManifest(ctx, f, testLocation, aug2026)
		require.NoError(t, err)
		assert.Equal(t, []string{
			"exports/nullstone-focus/data/BILLING_PERIOD=2026-08/nullstone-focus-00001.csv.gz",
			"exports/nullstone-focus/data/BILLING_PERIOD=2026-08/nullstone-focus-00002.csv.gz",
		}, m.DataFiles)
	})

	t.Run("missing month", func(t *testing.T) {
		_, err := ReadManifest(ctx, newFakeS3(), testLocation, aug2026)
		require.ErrorIs(t, err, ErrNoManifest)
	})
}

func TestListMonths(t *testing.T) {
	f := newFakeS3()
	f.put(testLocation.ManifestKey(sep2026), []byte(`{}`))
	f.put(testLocation.ManifestKey(aug2026), []byte(`{}`))
	f.put(testLocation.MetadataPrefix()+"BILLING_PERIOD=2026-08/nullstone-focus-create-table.sql", []byte{})
	f.put(testLocation.MetadataPrefix()+"BILLING_PERIOD=2026-07/20260701T000000Z-abc/nullstone-focus-Manifest.json", []byte(`{}`))

	months, err := ListMonths(context.Background(), f, testLocation)
	require.NoError(t, err)
	// the create-new execution folder for July is not an overwrite-mode partition manifest
	assert.Equal(t, []time.Time{aug2026, sep2026}, months)
}

func TestLocation(t *testing.T) {
	loc, err := ParseLocation(" s3://ns-billing/exports/nullstone-focus/ ")
	require.NoError(t, err)
	assert.Equal(t, testLocation, loc)
	assert.Equal(t, "nullstone-focus", loc.ExportName())
	assert.Equal(t, "s3://ns-billing/exports/nullstone-focus", loc.String())
	assert.Equal(t, "exports/nullstone-focus/metadata/BILLING_PERIOD=2026-08/nullstone-focus-Manifest.json", loc.ManifestKey(aug2026))
	assert.Equal(t, "exports/nullstone-focus/data/BILLING_PERIOD=2026-08/", loc.DataPrefix(aug2026))
	assert.Equal(t, "exports/nullstone-focus/metadata/", loc.MetadataPrefix())

	loc, err = ParseLocation("s3://bucket/export-only")
	require.NoError(t, err)
	assert.Equal(t, "export-only", loc.ExportName())

	for _, bad := range []string{"", "bucket/prefix", "https://bucket/prefix", "s3://bucket", "s3://bucket/"} {
		_, err := ParseLocation(bad)
		assert.Error(t, err, bad)
	}
}
