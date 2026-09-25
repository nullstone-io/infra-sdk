package aws_data_export

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func collectRows(t *testing.T, csv string) []Row {
	t.Helper()
	rows := make([]Row, 0)
	err := ReadCsv(strings.NewReader(csv), func(row Row) error {
		rows = append(rows, row)
		return nil
	})
	require.NoError(t, err)
	return rows
}

func TestReadCsv(t *testing.T) {
	t.Run("columns resolve by name, any order or case", func(t *testing.T) {
		rows := collectRows(t, "listcost,BILLEDCOST,ChargePeriodStart,ServiceName,ChargeCategory,BillingCurrency,ContractedCost,EffectiveCost,SubAccountId,ResourceId,Tags\n"+
			"10,8,2026-08-01T00:00:00Z,Amazon EC2,Usage,USD,9,8,111,i-1,\"{\"\"user:Stack\"\":\"\"core\"\"}\"\n")
		require.Len(t, rows, 1)
		assert.Equal(t, Row{
			ChargePeriodStart: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
			SubAccountId:      "111",
			ServiceName:       "Amazon EC2",
			ChargeCategory:    "Usage",
			ResourceId:        "i-1",
			BillingCurrency:   "USD",
			Tags:              map[string]string{"user:Stack": "core"},
			ListCost:          10, ContractedCost: 9, EffectiveCost: 8, BilledCost: 8,
		}, rows[0])
	})

	t.Run("optional columns may be absent", func(t *testing.T) {
		rows := collectRows(t, "ChargePeriodStart,ServiceName,ChargeCategory,BillingCurrency,ListCost,ContractedCost,EffectiveCost,BilledCost\n"+
			"2026-08-01T00:00:00Z,Tax,Tax,USD,1,1,1,1\n")
		require.Len(t, rows, 1)
		assert.Empty(t, rows[0].ResourceId)
		assert.Nil(t, rows[0].Tags)
	})

	t.Run("required column missing", func(t *testing.T) {
		err := ReadCsv(strings.NewReader("ChargePeriodStart,ServiceName,BillingCurrency,BilledCost\n"), func(Row) error { return nil })
		require.Error(t, err)
		assert.Contains(t, err.Error(), "chargecategory")
	})

	t.Run("empty stream and header-only stream yield no rows", func(t *testing.T) {
		assert.Empty(t, collectRows(t, ""))
		assert.Empty(t, collectRows(t, "ChargePeriodStart,ServiceName,ChargeCategory,BillingCurrency,ListCost,ContractedCost,EffectiveCost,BilledCost\n"))
	})

	t.Run("empty costs read as zero", func(t *testing.T) {
		rows := collectRows(t, "ChargePeriodStart,ServiceName,ChargeCategory,BillingCurrency,ListCost,ContractedCost,EffectiveCost,BilledCost\n"+
			"2026-08-01T00:00:00Z,S,Usage,USD,,,,3.25\n")
		assert.Equal(t, 0.0, rows[0].ListCost)
		assert.Equal(t, 3.25, rows[0].BilledCost)
	})

	t.Run("bad cost and bad timestamp report the line", func(t *testing.T) {
		header := "ChargePeriodStart,ServiceName,ChargeCategory,BillingCurrency,ListCost,ContractedCost,EffectiveCost,BilledCost\n"
		err := ReadCsv(strings.NewReader(header+"2026-08-01T00:00:00Z,S,Usage,USD,x,0,0,0\n"), func(Row) error { return nil })
		require.Error(t, err)
		assert.Contains(t, err.Error(), "line 2")
		err = ReadCsv(strings.NewReader(header+"2026-08-01T00:00:00Z,S,Usage,USD,0,0,0,0\nnot-a-date,S,Usage,USD,0,0,0,0\n"), func(Row) error { return nil })
		require.Error(t, err)
		assert.Contains(t, err.Error(), "line 3")
	})

	t.Run("callback errors stop the read", func(t *testing.T) {
		header := "ChargePeriodStart,ServiceName,ChargeCategory,BillingCurrency,ListCost,ContractedCost,EffectiveCost,BilledCost\n"
		err := ReadCsv(strings.NewReader(header+"2026-08-01T00:00:00Z,S,Usage,USD,0,0,0,0\n"), func(Row) error { return assert.AnError })
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestParseTimestamp(t *testing.T) {
	want := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	for _, raw := range []string{
		"2026-08-01T00:00:00Z", "2026-08-01T00:00:00.000Z", "2026-08-01 00:00:00.000", "2026-08-01 00:00:00", "2026-08-01", "2026-08-01T00:00:00+00:00",
	} {
		got, err := parseTimestamp(raw)
		require.NoError(t, err, raw)
		assert.Equal(t, want, got, raw)
	}
	_, err := parseTimestamp("08/01/2026")
	assert.Error(t, err)
}

func TestParseTags(t *testing.T) {
	for _, raw := range []string{"", "{}", "  "} {
		tags, err := parseTags(raw)
		require.NoError(t, err)
		assert.Nil(t, tags)
	}
	tags, err := parseTags(`{"user:Stack":"core","aws:createdBy":"x"}`)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"user:Stack": "core", "aws:createdBy": "x"}, tags)
	_, err = parseTags(`Stack=core`)
	assert.Error(t, err)
}

func TestTagValue(t *testing.T) {
	v, ok := TagValue(map[string]string{"user:Stack": "core"}, "Stack")
	assert.True(t, ok)
	assert.Equal(t, "core", v)
	v, ok = TagValue(map[string]string{"Stack": "core"}, "Stack")
	assert.True(t, ok)
	assert.Equal(t, "core", v)
	_, ok = TagValue(nil, "Stack")
	assert.False(t, ok)
}

func TestReadRows(t *testing.T) {
	ctx := context.Background()
	f := fakeExport(t)
	keys := []string{
		testLocation.DataPrefix(aug2026) + "nullstone-focus-00001.csv.gz",
		testLocation.DataPrefix(aug2026) + "nullstone-focus-00002.csv.gz", // zero bytes: a spare chunk
	}
	n := 0
	require.NoError(t, ReadRows(ctx, f, testLocation.Bucket, keys, func(Row) error { n++; return nil }))
	assert.Equal(t, 9, n)

	err := ReadRows(ctx, f, testLocation.Bucket, []string{"x.snappy.parquet"}, func(Row) error { return nil })
	assert.ErrorIs(t, err, ErrUnsupportedFormat)

	err = ReadRows(ctx, f, testLocation.Bucket, []string{"missing.csv.gz"}, func(Row) error { return nil })
	assert.Error(t, err)
}
