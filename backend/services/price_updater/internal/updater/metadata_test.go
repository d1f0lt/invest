package updater

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"invest/backend/services/price_updater/internal/dohod"
	"invest/backend/services/price_updater/internal/moex"
	"invest/backend/services/price_updater/internal/storage"
)

type metadataFake struct {
	targets        []storage.MetadataTarget
	descErr        error
	issuerErr      error
	dividendErr    error
	infoSaved      int
	dividendsSaved int
	descCalls      int
	dividendCalls  int
	values         map[string]string
	issuer         string
	dividends      []dohod.Dividend
}

func (f *metadataFake) MetadataTargets(context.Context, []string) ([]storage.MetadataTarget, error) {
	return f.targets, nil
}
func (f *metadataFake) Description(context.Context, string) ([]moex.DescriptionField, error) {
	f.descCalls++
	return []moex.DescriptionField{{Name: "NAME", Value: "Сбербанк"}}, f.descErr
}
func (f *metadataFake) EmitterTitle(context.Context, string) (string, error) {
	return "ПАО Сбербанк", f.issuerErr
}
func (f *metadataFake) Dividends(context.Context, string) ([]dohod.Dividend, error) {
	f.dividendCalls++
	return []dohod.Dividend{{RegistryCloseDate: "2027-07-20", Value: 44.53, Currency: "RUB", Forecast: true}}, f.dividendErr
}
func (f *metadataFake) SaveSecurityMetadata(_ context.Context, _ string, values map[string]string, issuer string) error {
	f.infoSaved++
	f.values = values
	f.issuer = issuer
	return nil
}
func (f *metadataFake) SaveDividends(_ context.Context, _ string, dividends []dohod.Dividend) error {
	f.dividendsSaved++
	f.dividends = dividends
	return nil
}
func metadataJob(f *metadataFake) *MetadataJob {
	return &MetadataJob{Client: f, Dividends: f, Store: f, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestMetadataJobPersistsIndependentSnapshots(t *testing.T) {
	f := &metadataFake{targets: []storage.MetadataTarget{{SecID: "SBER", InfoDue: true, DividendsDue: true}}}
	metadataJob(f).RunOnce(context.Background())
	if f.infoSaved != 1 || f.dividendsSaved != 1 || f.values["NAME"] != "Сбербанк" || f.issuer != "ПАО Сбербанк" || len(f.dividends) != 1 || !f.dividends[0].Forecast {
		t.Fatalf("result=%+v", f)
	}
}

func TestMetadataJobKeepsOldInfoOnPartialFailureAndStillLoadsDividends(t *testing.T) {
	for _, part := range []string{"description", "issuer"} {
		t.Run(part, func(t *testing.T) {
			f := &metadataFake{targets: []storage.MetadataTarget{{SecID: "SBER", InfoDue: true, DividendsDue: true}}}
			if part == "description" {
				f.descErr = errors.New("timeout")
			} else {
				f.issuerErr = errors.New("timeout")
			}
			metadataJob(f).RunOnce(context.Background())
			if f.infoSaved != 0 || f.dividendsSaved != 1 {
				t.Fatalf("result=%+v", f)
			}
		})
	}
}

func TestMetadataJobSkipsFreshInfoAndKeepsOldDividendsOnFailure(t *testing.T) {
	f := &metadataFake{targets: []storage.MetadataTarget{{SecID: "SBER", DividendsDue: true}}, dividendErr: errors.New("timeout")}
	metadataJob(f).RunOnce(context.Background())
	if f.descCalls != 0 || f.dividendCalls != 1 || f.dividendsSaved != 0 {
		t.Fatalf("result=%+v", f)
	}
}
