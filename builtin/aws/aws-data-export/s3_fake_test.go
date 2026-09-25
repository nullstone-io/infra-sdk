package aws_data_export

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/require"
)

// fakeS3 serves objects from memory. Missing keys answer the way S3 does: NotFound on head,
// NoSuchKey on get.
type fakeS3 struct {
	objects map[string][]byte
}

func newFakeS3() *fakeS3 { return &fakeS3{objects: map[string][]byte{}} }

func (f *fakeS3) put(key string, body []byte) { f.objects[key] = body }

func (f *fakeS3) etag(body []byte) *string {
	return ptr(fmt.Sprintf("%q", fmt.Sprintf("%x", md5.Sum(body))))
}

func (f *fakeS3) HeadObject(_ context.Context, in *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	body, ok := f.objects[*in.Key]
	if !ok {
		return nil, &s3types.NotFound{}
	}
	return &s3.HeadObjectOutput{ETag: f.etag(body), ContentLength: ptr(int64(len(body)))}, nil
}

func (f *fakeS3) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	body, ok := f.objects[*in.Key]
	if !ok {
		return nil, &s3types.NoSuchKey{}
	}
	return &s3.GetObjectOutput{
		Body:          io.NopCloser(bytes.NewReader(body)),
		ETag:          f.etag(body),
		ContentLength: ptr(int64(len(body))),
	}, nil
}

func (f *fakeS3) ListObjectsV2(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	keys := make([]string, 0)
	for key := range f.objects {
		if strings.HasPrefix(key, unptr(in.Prefix)) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	out := &s3.ListObjectsV2Output{}
	for _, key := range keys {
		out.Contents = append(out.Contents, s3types.Object{Key: ptr(key), Size: ptr(int64(len(f.objects[key])))})
	}
	return out, nil
}

func gzipBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err := gz.Write(raw)
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("fixtures", name))
	require.NoError(t, err)
	return raw
}

var (
	testLocation = Location{Bucket: "ns-billing", Prefix: "exports/nullstone-focus"}
	aug2026      = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	sep2026      = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
)

// fakeExport serves the August 2026 fixture month: its manifest, one data chunk with the
// fixture rows, and a zero-byte spare chunk (what a shrinking refresh leaves behind).
func fakeExport(t *testing.T) *fakeS3 {
	t.Helper()
	f := newFakeS3()
	f.put(testLocation.ManifestKey(aug2026), fixture(t, "nullstone-focus-Manifest.json"))
	f.put(testLocation.DataPrefix(aug2026)+"nullstone-focus-00001.csv.gz", gzipBytes(t, fixture(t, "nullstone-focus-2026-08.csv")))
	f.put(testLocation.DataPrefix(aug2026)+"nullstone-focus-00002.csv.gz", []byte{})
	return f
}
