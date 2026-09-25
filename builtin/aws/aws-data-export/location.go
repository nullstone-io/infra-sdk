package aws_data_export

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Location identifies one AWS Data Export in S3. Data Exports deliver to
//
//	s3://<bucket>/<prefix>/<export-name>/data/BILLING_PERIOD=YYYY-MM/<export-name>-NNNNN.csv.gz
//	s3://<bucket>/<prefix>/<export-name>/metadata/BILLING_PERIOD=YYYY-MM/<export-name>-Manifest.json
//
// so a Location is the bucket plus everything up to and including the export name. Users
// configure it as a single URI, s3://<bucket>/<prefix>/<export-name>.
type Location struct {
	Bucket string
	// Prefix is "<prefix>/<export-name>" with no leading or trailing slash.
	Prefix string
}

// ParseLocation parses s3://<bucket>/<prefix>/<export-name>.
func ParseLocation(uri string) (Location, error) {
	u, err := url.Parse(strings.TrimSpace(uri))
	if err != nil || u.Scheme != "s3" || u.Host == "" {
		return Location{}, fmt.Errorf("data export path must look like s3://<bucket>/<prefix>/<export-name>")
	}
	prefix := strings.Trim(u.Path, "/")
	if prefix == "" {
		return Location{}, fmt.Errorf("data export path must include the export prefix and name after the bucket")
	}
	return Location{Bucket: u.Host, Prefix: prefix}, nil
}

func (l Location) String() string {
	return fmt.Sprintf("s3://%s/%s", l.Bucket, l.Prefix)
}

// ExportName is the last segment of the prefix, which Data Exports uses to name every file.
func (l Location) ExportName() string {
	if i := strings.LastIndex(l.Prefix, "/"); i >= 0 {
		return l.Prefix[i+1:]
	}
	return l.Prefix
}

// Partition names the billing-period partition directory for a month.
func Partition(month time.Time) string {
	return "BILLING_PERIOD=" + month.UTC().Format("2006-01")
}

// MetadataPrefix is the key prefix under which every month's manifest lives.
func (l Location) MetadataPrefix() string {
	return l.Prefix + "/metadata/"
}

// ManifestKey is the object key of a month's manifest (overwrite delivery mode).
func (l Location) ManifestKey(month time.Time) string {
	return fmt.Sprintf("%s/metadata/%s/%s-Manifest.json", l.Prefix, Partition(month), l.ExportName())
}

// DataPrefix is the key prefix of a month's data files (overwrite delivery mode).
func (l Location) DataPrefix(month time.Time) string {
	return fmt.Sprintf("%s/data/%s/", l.Prefix, Partition(month))
}
