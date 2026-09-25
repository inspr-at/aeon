// SPDX-License-Identifier: AGPL-3.0-only
package offers

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxFile = 8 << 20
const maxBundle = 64 << 20

var customerFile = regexp.MustCompile(`^customer-([0-9]+)\.json$`)
var offerFile = regexp.MustCompile(`^offer-([0-9]+)\.json$`)

// Bundle is the EQ0 GET export. Settings and branding are required evidence,
// but quote content comes from each offer's own frozen document snapshot.
type Bundle struct {
	Customers []Customer
	Contacts  []Contact
	Offers    []Offer
	Settings  json.RawMessage
	Branding  json.RawMessage
}

type Customer struct {
	ID                 int64       `json:"id"`
	Name               string      `json:"name"`
	CustomerNo         string      `json:"customer_no"`
	Industry           string      `json:"industry"`
	Domain             string      `json:"domain"`
	ExternalProvider   string      `json:"external_provider"`
	ExternalID         string      `json:"external_id"`
	ExternalURL        string      `json:"external_url"`
	EmployeeCount      *int64      `json:"employee_count"`
	AnnualRevenueCents *int64      `json:"annual_revenue_cents"`
	RateHourly         json.Number `json:"rate_hourly"`
	RateLP             json.Number `json:"rate_lp"`
	Address            string      `json:"address"`
	Country            string      `json:"country"`
	Website            string      `json:"website"`
	Phone              string      `json:"phone"`
	Description        string      `json:"description"`
	Notes              string      `json:"notes"`
	VATID              string      `json:"vat_id"`
	TaxID              string      `json:"tax_id"`
	CompanyRegisterNo  string      `json:"company_register_number"`
	BillingStreet      string      `json:"billing_address_street"`
	BillingZIP         string      `json:"billing_address_zip"`
	BillingCity        string      `json:"billing_address_city"`
	BillingCountry     string      `json:"billing_address_country"`
	VisitingStreet     string      `json:"visit_address_street"`
	VisitingZIP        string      `json:"visit_address_zip"`
	UpdatedAt          string      `json:"updated_at"`
}
type Contact struct {
	ID               int64  `json:"id"`
	CustomerID       int64  `json:"customer_id"`
	Name             string `json:"name"`
	Email            string `json:"email"`
	Phone            string `json:"phone"`
	Role             string `json:"role"`
	Notes            string `json:"notes"`
	ExternalProvider string `json:"external_provider"`
	ExternalID       string `json:"external_id"`
	ExternalURL      string `json:"external_url"`
	IsPrimary        bool   `json:"is_primary"`
	UpdatedAt        string `json:"updated_at"`
}
type Offer struct {
	ID         int64           `json:"id"`
	CustomerID int64           `json:"customer_id"`
	OfferNo    string          `json:"offer_no"`
	Revision   int64           `json:"revision"`
	Status     string          `json:"status"`
	SentAt     string          `json:"sent_at"`
	Document   json.RawMessage `json:"document"`
}

// ReadDir and ReadTar accept the same flat EQ0 names, optionally below raw/.
// They never follow links or read outside the supplied bundle.
func ReadDir(path string) (Bundle, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return Bundle{}, err
	}
	for _, entry := range entries {
		if entry.Name() == "raw" && entry.IsDir() {
			return ReadDir(filepath.Join(path, "raw"))
		}
	}
	files := map[string][]byte{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return Bundle{}, fmt.Errorf("symlink in bundle: %s", entry.Name())
		}
		info, err := entry.Info()
		if err != nil {
			return Bundle{}, err
		}
		if info.Size() > maxFile {
			return Bundle{}, fmt.Errorf("bundle file too large: %s", entry.Name())
		}
		data, err := os.ReadFile(filepath.Join(path, entry.Name()))
		if err != nil {
			return Bundle{}, err
		}
		files[entry.Name()] = data
	}
	return parse(files)
}

func ReadTar(r io.Reader) (Bundle, error) {
	files := map[string][]byte{}
	tr := tar.NewReader(io.LimitReader(r, maxBundle+1))
	var total int64
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Bundle{}, err
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		if h.Typeflag != tar.TypeReg || h.Size < 0 || h.Size > maxFile {
			return Bundle{}, fmt.Errorf("invalid bundle entry: %s", h.Name)
		}
		name := filepath.ToSlash(filepath.Clean(h.Name))
		if strings.HasPrefix(name, "../") || filepath.IsAbs(name) || strings.Contains(name, "\\") {
			return Bundle{}, fmt.Errorf("invalid bundle path: %s", h.Name)
		}
		base := filepath.Base(name)
		if !strings.HasSuffix(base, ".json") {
			continue
		}
		if _, exists := files[base]; exists {
			return Bundle{}, fmt.Errorf("duplicate bundle entry: %s", base)
		}
		data, err := io.ReadAll(io.LimitReader(tr, maxFile+1))
		if err != nil {
			return Bundle{}, err
		}
		if int64(len(data)) != h.Size {
			return Bundle{}, fmt.Errorf("truncated bundle entry: %s", base)
		}
		total += int64(len(data))
		if total > maxBundle {
			return Bundle{}, errors.New("bundle too large")
		}
		files[base] = data
	}
	return parse(files)
}

func parse(files map[string][]byte) (Bundle, error) {
	var b Bundle
	if !json.Valid(files["offer-settings.json"]) || !json.Valid(files["branding.json"]) {
		return b, errors.New("bundle needs offer-settings.json and branding.json")
	}
	b.Settings, b.Branding = files["offer-settings.json"], files["branding.json"]
	ids := make([]int64, 0)
	for name := range files {
		if m := customerFile.FindStringSubmatch(name); m != nil {
			id, _ := strconv.ParseInt(m[1], 10, 64)
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	seenOffers := map[int64]bool{}
	for _, id := range ids {
		var c Customer
		if err := decode(files[fmt.Sprintf("customer-%d.json", id)], &c); err != nil {
			return b, err
		}
		if c.ID != id || c.ID <= 0 || strings.TrimSpace(c.Name) == "" {
			return b, fmt.Errorf("invalid customer %d", id)
		}
		b.Customers = append(b.Customers, c)
		var contacts []Contact
		if err := decode(files[fmt.Sprintf("customer-%d-contacts.json", id)], &contacts); err != nil {
			return b, err
		}
		for _, v := range contacts {
			if v.ID <= 0 || v.CustomerID != id || strings.TrimSpace(v.Name) == "" {
				return b, fmt.Errorf("invalid contact for customer %d", id)
			}
			b.Contacts = append(b.Contacts, v)
		}
		var summaries []Offer
		if err := decode(files[fmt.Sprintf("customer-%d-offers.json", id)], &summaries); err != nil {
			return b, err
		}
		for _, summary := range summaries {
			var o Offer
			if err := decode(files[fmt.Sprintf("offer-%d.json", summary.ID)], &o); err != nil {
				return b, err
			}
			separate := files[fmt.Sprintf("offer-%d-document.json", summary.ID)]
			if len(separate) == 0 {
				return b, fmt.Errorf("missing document for offer %d", summary.ID)
			}
			var embedded, extracted any
			if err := decode(o.Document, &embedded); err != nil {
				return b, err
			}
			if err := decode(separate, &extracted); err != nil {
				return b, err
			}
			if !reflect.DeepEqual(embedded, extracted) {
				return b, fmt.Errorf("document mismatch for offer %d", summary.ID)
			}
			if o.ID <= 0 || o.ID != summary.ID || o.CustomerID != id || seenOffers[o.ID] || o.Revision < 1 || strings.TrimSpace(o.OfferNo) == "" {
				return b, fmt.Errorf("invalid offer %d", o.ID)
			}
			seenOffers[o.ID] = true
			b.Offers = append(b.Offers, o)
		}
	}
	if len(b.Customers) == 0 {
		return b, errors.New("bundle has no customers")
	}
	for name := range files {
		if m := offerFile.FindStringSubmatch(name); m != nil {
			id, _ := strconv.ParseInt(m[1], 10, 64)
			if !seenOffers[id] {
				return b, fmt.Errorf("unlisted offer %d", id)
			}
		}
	}
	return b, nil
}
func decode(raw []byte, target any) error {
	if len(raw) == 0 {
		return errors.New("missing bundle record")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(target); err != nil {
		return err
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("trailing bundle JSON")
	}
	return nil
}
func revisionTime(value string) (int64, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t.Unix(), nil
		}
	}
	return 0, fmt.Errorf("invalid source revision timestamp")
}
