package aws_data_export

import (
	"compress/gzip"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// ErrUnsupportedFormat reports a data file this package cannot read (a parquet export).
var ErrUnsupportedFormat = errors.New("data export must be delivered as gzip/csv (TEXT_OR_CSV + GZIP); parquet is not supported")

// Row is the projection of one FOCUS 1.2 line item this package uses.
type Row struct {
	ChargePeriodStart time.Time
	SubAccountId      string
	ServiceName       string
	ChargeCategory    string
	ResourceId        string
	BillingCurrency   string
	// Tags holds the row's tags as delivered: user tags (which AWS may prefix with "user:"),
	// AWS tags and cost-category tags. Use TagValue to look one up.
	Tags map[string]string

	ListCost       float64
	ContractedCost float64
	EffectiveCost  float64
	BilledCost     float64
}

// TagValue looks up a user tag by its bare key, tolerating the "user:" prefix AWS puts on
// user-defined tags in some outputs.
func TagValue(tags map[string]string, key string) (string, bool) {
	if v, ok := tags[key]; ok {
		return v, true
	}
	if v, ok := tags["user:"+key]; ok {
		return v, true
	}
	return "", false
}

// focus column names, matched case-insensitively against the CSV header
const (
	colChargePeriodStart = "chargeperiodstart"
	colSubAccountId      = "subaccountid"
	colServiceName       = "servicename"
	colChargeCategory    = "chargecategory"
	colResourceId        = "resourceid"
	colBillingCurrency   = "billingcurrency"
	colTags              = "tags"
	colListCost          = "listcost"
	colContractedCost    = "contractedcost"
	colEffectiveCost     = "effectivecost"
	colBilledCost        = "billedcost"
)

var requiredColumns = []string{
	colChargePeriodStart, colServiceName, colChargeCategory, colBillingCurrency,
	colListCost, colContractedCost, colEffectiveCost, colBilledCost,
}

// ReadRows streams every data file, calling fn once per line item. Files are read straight
// from S3 through gzip and csv; nothing is buffered beyond one record. Zero-byte files (the
// spare chunks Data Exports leaves behind when a refresh shrinks) are skipped.
func ReadRows(ctx context.Context, api S3API, bucket string, keys []string, fn func(Row) error) error {
	for _, key := range keys {
		if !IsCsvGzip(key) {
			return fmt.Errorf("%w: %s", ErrUnsupportedFormat, key)
		}
		if err := readFile(ctx, api, bucket, key, fn); err != nil {
			return fmt.Errorf("error reading s3://%s/%s: %w", bucket, key, err)
		}
	}
	return nil
}

func readFile(ctx context.Context, api S3API, bucket, key string, fn func(Row) error) error {
	out, err := api.GetObject(ctx, &s3.GetObjectInput{Bucket: ptr(bucket), Key: ptr(key)})
	if err != nil {
		return err
	}
	defer out.Body.Close()
	if out.ContentLength != nil && *out.ContentLength == 0 {
		return nil
	}
	gz, err := gzip.NewReader(out.Body)
	if err != nil {
		if errors.Is(err, io.EOF) {
			// empty object without a content length
			return nil
		}
		return fmt.Errorf("error opening gzip: %w", err)
	}
	defer gz.Close()
	return ReadCsv(gz, fn)
}

// ReadCsv parses one FOCUS csv stream. Columns are resolved from the header row by name so
// column order does not matter.
func ReadCsv(r io.Reader, fn func(Row) error) error {
	cr := csv.NewReader(r)
	cr.ReuseRecord = true
	cr.FieldsPerRecord = -1

	header, err := cr.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("error reading header: %w", err)
	}
	cols := map[string]int{}
	for i, name := range header {
		cols[strings.ToLower(strings.TrimSpace(name))] = i
	}
	for _, name := range requiredColumns {
		if _, ok := cols[name]; !ok {
			return fmt.Errorf("column %q is missing; is this a FOCUS 1.2 export?", name)
		}
	}

	field := func(rec []string, name string) string {
		i, ok := cols[name]
		if !ok || i >= len(rec) {
			return ""
		}
		return rec[i]
	}

	for line := 2; ; line++ {
		rec, err := cr.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("line %d: %w", line, err)
		}
		row := Row{
			SubAccountId:    field(rec, colSubAccountId),
			ServiceName:     field(rec, colServiceName),
			ChargeCategory:  field(rec, colChargeCategory),
			ResourceId:      field(rec, colResourceId),
			BillingCurrency: field(rec, colBillingCurrency),
		}
		if row.ChargePeriodStart, err = parseTimestamp(field(rec, colChargePeriodStart)); err != nil {
			return fmt.Errorf("line %d: invalid ChargePeriodStart: %w", line, err)
		}
		if row.Tags, err = parseTags(field(rec, colTags)); err != nil {
			return fmt.Errorf("line %d: invalid Tags: %w", line, err)
		}
		for _, cost := range []struct {
			name string
			dest *float64
		}{
			{colListCost, &row.ListCost},
			{colContractedCost, &row.ContractedCost},
			{colEffectiveCost, &row.EffectiveCost},
			{colBilledCost, &row.BilledCost},
		} {
			if *cost.dest, err = parseCost(field(rec, cost.name)); err != nil {
				return fmt.Errorf("line %d: invalid %s: %w", line, cost.name, err)
			}
		}
		if err := fn(row); err != nil {
			return err
		}
	}
}

var timestampLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05.000Z",
	"2006-01-02 15:04:05.000",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

func parseTimestamp(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	for _, layout := range timestampLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized timestamp %q", raw)
}

// parseTags decodes the Tags column, a JSON object in csv output. Empty means no tags.
func parseTags(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return nil, nil
	}
	tags := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		return nil, err
	}
	return tags, nil
}

func parseCost(raw string) (float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	return strconv.ParseFloat(raw, 64)
}
