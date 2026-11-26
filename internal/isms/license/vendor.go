package license

import (
	docxlic "github.com/unidoc/unioffice/v2/common/license"
	pdflic "github.com/unidoc/unipdf/v5/common/license"
)

// The two real adapters. Everything vendor-specific lives in this file; the
// rest of the package only sees the lib interface.

type pdfLib struct{}

func (pdfLib) Name() string                    { return "PDF" }
func (pdfLib) Product() string                 { return "UniPDF" }
func (pdfLib) Format() string                  { return "pdf" }
func (pdfLib) VendorVar() string               { return "UNIPDF_LICENSE_PATH" }
func (pdfLib) VendorCustomerVar() string       { return "UNIPDF_CUSTOMER_NAME" }
func (pdfLib) KeyVar() string                  { return "ISMS_UNIPDF_LICENSE_KEY" }
func (pdfLib) CustomerVar() string             { return "ISMS_UNIPDF_LICENSE_CUSTOMER_NAME" }
func (pdfLib) SetLicenseKey(k, c string) error { return pdflic.SetLicenseKey(k, c) }
func (pdfLib) SetMeteredKey(k string) error    { return pdflic.SetMeteredKey(k) }
func (pdfLib) SetPersistentCache(v bool)       { pdflic.SetMeteredKeyPersistentCache(v) }
func (pdfLib) Licensed() bool {
	k := pdflic.GetLicenseKey()
	return k != nil && k.Tier != pdflic.LicenseTierUnlicensed
}

type docxLib struct{}

func (docxLib) Name() string                    { return "DOCX" }
func (docxLib) Product() string                 { return "UniOffice" }
func (docxLib) Format() string                  { return "docx" }
func (docxLib) VendorVar() string               { return "UNIOFFICE_LICENSE_PATH" }
func (docxLib) VendorCustomerVar() string       { return "UNIOFFICE_CUSTOMER_NAME" }
func (docxLib) KeyVar() string                  { return "ISMS_UNIOFFICE_LICENSE_KEY" }
func (docxLib) CustomerVar() string             { return "ISMS_UNIOFFICE_LICENSE_CUSTOMER_NAME" }
func (docxLib) SetLicenseKey(k, c string) error { return docxlic.SetLicenseKey(k, c) }
func (docxLib) SetMeteredKey(k string) error    { return docxlic.SetMeteredKey(k) }
func (docxLib) SetPersistentCache(v bool)       { docxlic.SetMeteredKeyPersistentCache(v) }
func (docxLib) Licensed() bool {
	k := docxlic.GetLicenseKey()
	return k != nil && k.Tier != docxlic.LicenseTierUnlicensed
}

func realLibs() []lib { return []lib{pdfLib{}, docxLib{}} }
