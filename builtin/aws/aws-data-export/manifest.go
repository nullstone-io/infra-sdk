package aws_data_export

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// S3API is the slice of the S3 client this package needs, so callers (and tests) can supply
// their own client.
type S3API interface {
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

// ErrNoManifest reports that a billing month has no manifest: the export did not exist yet, or
// AWS has not delivered that month.
var ErrNoManifest = errors.New("no data export manifest for this billing month")

// Manifest is the metadata Data Exports writes next to each billing month's data files. It is
// only written once every data file of a refresh has landed, so it is the unit of work: read
// it, then read exactly the files it lists.
type Manifest struct {
	ExportName string
	TableName  string
	// Columns lists the column names in the data files, when the manifest reports them.
	Columns []string
	// DataFiles are the object keys of the month's data files (bucket-relative).
	DataFiles []string
	// ETag identifies this refresh of the manifest; it changes whenever AWS re-delivers the month.
	ETag string
}

// rawManifest decodes the manifest leniently. AWS documents what the manifest contains (columns,
// data file paths, billing period) but not its exact field names, and CUR 2.0 writes the file
// list and columns as objects. Entries are accepted as strings or as objects carrying a
// key/path/name. Unknown fields are ignored.
type rawManifest struct {
	ExportName string       `json:"exportName"`
	TableName  string       `json:"tableName"`
	Columns    []rawNamed   `json:"columns"`
	DataFiles  []rawDataKey `json:"dataFiles"`
}

type rawNamed struct{ Name string }

func (n *rawNamed) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		n.Name = s
		return nil
	}
	var obj struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	n.Name = obj.Name
	return nil
}

type rawDataKey struct{ Key string }

func (k *rawDataKey) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		k.Key = s
		return nil
	}
	var obj struct {
		Key  string `json:"key"`
		Path string `json:"path"`
		Uri  string `json:"uri"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	switch {
	case obj.Key != "":
		k.Key = obj.Key
	case obj.Path != "":
		k.Key = obj.Path
	default:
		k.Key = obj.Uri
	}
	return nil
}

// ReadManifest fetches and parses a month's manifest. ErrNoManifest when the month has none.
// When the manifest lists no data files, the month's data partition is listed instead.
func ReadManifest(ctx context.Context, api S3API, loc Location, month time.Time) (*Manifest, error) {
	key := loc.ManifestKey(month)
	out, err := api.GetObject(ctx, &s3.GetObjectInput{Bucket: ptr(loc.Bucket), Key: ptr(key)})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNoManifest
		}
		return nil, fmt.Errorf("error reading manifest s3://%s/%s: %w", loc.Bucket, key, err)
	}
	defer out.Body.Close()
	raw, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading manifest s3://%s/%s: %w", loc.Bucket, key, err)
	}
	m, err := ParseManifest(raw)
	if err != nil {
		return nil, fmt.Errorf("error parsing manifest s3://%s/%s: %w", loc.Bucket, key, err)
	}
	m.ETag = unptr(out.ETag)
	for i, file := range m.DataFiles {
		m.DataFiles[i] = normalizeKey(loc.Bucket, file)
	}
	if len(m.DataFiles) == 0 {
		m.DataFiles, err = listDataFiles(ctx, api, loc, month)
		if err != nil {
			return nil, err
		}
	}
	return m, nil
}

// ParseManifest decodes manifest JSON.
func ParseManifest(raw []byte) (*Manifest, error) {
	var rm rawManifest
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&rm); err != nil {
		return nil, err
	}
	m := &Manifest{
		ExportName: rm.ExportName,
		TableName:  rm.TableName,
		Columns:    make([]string, 0, len(rm.Columns)),
		DataFiles:  make([]string, 0, len(rm.DataFiles)),
	}
	for _, col := range rm.Columns {
		if col.Name != "" {
			m.Columns = append(m.Columns, col.Name)
		}
	}
	for _, file := range rm.DataFiles {
		if file.Key != "" {
			m.DataFiles = append(m.DataFiles, file.Key)
		}
	}
	return m, nil
}

// normalizeKey turns an absolute s3:// URI or a leading-slash path into a bucket-relative key.
func normalizeKey(bucket, key string) string {
	key = strings.TrimPrefix(key, "s3://"+bucket+"/")
	return strings.TrimPrefix(key, "/")
}

func listDataFiles(ctx context.Context, api S3API, loc Location, month time.Time) ([]string, error) {
	prefix := loc.DataPrefix(month)
	keys := make([]string, 0)
	var token *string
	for {
		out, err := api.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            ptr(loc.Bucket),
			Prefix:            ptr(prefix),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("error listing data files s3://%s/%s: %w", loc.Bucket, prefix, err)
		}
		for _, obj := range out.Contents {
			key := unptr(obj.Key)
			if IsDataFile(key) {
				keys = append(keys, key)
			}
		}
		if out.NextContinuationToken == nil || *out.NextContinuationToken == "" {
			break
		}
		token = out.NextContinuationToken
	}
	sort.Strings(keys)
	return keys, nil
}

// HasMonth reports whether a billing month has a manifest, and its ETag when it does.
func HasMonth(ctx context.Context, api S3API, loc Location, month time.Time) (string, bool, error) {
	key := loc.ManifestKey(month)
	out, err := api.HeadObject(ctx, &s3.HeadObjectInput{Bucket: ptr(loc.Bucket), Key: ptr(key)})
	if err != nil {
		if isNotFound(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("error checking manifest s3://%s/%s: %w", loc.Bucket, key, err)
	}
	return unptr(out.ETag), true, nil
}

var manifestKeyPattern = regexp.MustCompile(`/metadata/BILLING_PERIOD=(\d{4}-\d{2})/[^/]+-Manifest\.json$`)

// ListMonths lists the billing months that have a manifest, oldest first.
func ListMonths(ctx context.Context, api S3API, loc Location) ([]time.Time, error) {
	prefix := loc.MetadataPrefix()
	months := make([]time.Time, 0)
	seen := map[string]bool{}
	var token *string
	for {
		out, err := api.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            ptr(loc.Bucket),
			Prefix:            ptr(prefix),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("error listing manifests s3://%s/%s: %w", loc.Bucket, prefix, err)
		}
		for _, obj := range out.Contents {
			match := manifestKeyPattern.FindStringSubmatch(unptr(obj.Key))
			if match == nil || seen[match[1]] {
				continue
			}
			month, err := time.Parse("2006-01", match[1])
			if err != nil {
				continue
			}
			seen[match[1]] = true
			months = append(months, month)
		}
		if out.NextContinuationToken == nil || *out.NextContinuationToken == "" {
			break
		}
		token = out.NextContinuationToken
	}
	sort.Slice(months, func(i, j int) bool { return months[i].Before(months[j]) })
	return months, nil
}

// IsDataFile reports whether a key is a Data Exports data chunk (either delivery format).
func IsDataFile(key string) bool {
	return IsCsvGzip(key) || strings.HasSuffix(key, ".snappy.parquet")
}

// IsCsvGzip reports whether a data file is in the gzip/csv format this package reads.
func IsCsvGzip(key string) bool {
	return strings.HasSuffix(key, ".csv.gz")
}

func isNotFound(err error) bool {
	var noSuchKey *s3types.NoSuchKey
	var notFound *s3types.NotFound
	if errors.As(err, &noSuchKey) || errors.As(err, &notFound) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound":
			return true
		}
	}
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) && respErr.HTTPStatusCode() == http.StatusNotFound {
		return true
	}
	return false
}

func ptr[T any](v T) *T { return &v }

func unptr[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
