package invoices

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"strconv"
	"time"

	"github.com/byblix/byrd-accounting/storage"
	"github.com/jung-kurt/gofpdf"
	"github.com/leekchan/accounting"
)

const (
	denmark                = "Denmark"
	danmark                = "Danmark"
	other                  = "Other"
	dkk                    = "DKK"
	eur                    = "EUR"
	month                  = "month"
	year                   = "year"
	productLineNumber      = 1
	productSortKey         = 1
	photographerCut        = 15
	unlimitedAmountCredits = 0
	euroToDKKPrice         = 7.425
	productPAYG            = "22"
)

// PDFLine -
type PDFLine struct {
	InvoiceNum                               int
	Recipient                                *Recipient
	Date, Period                             string
	MaxSellerCut, MinByrdInc, NetAmount, VAT float64
}

// TotalVals -
type TotalVals struct {
	TotalSellerCut, TotalByrdInc, TotalNetAmount, TotalVAT float64
}

var ftrHdrSizes = []float64{20, 20, 50, 20, 20, 30, 30, 30, 30}
var noPlatForm = map[string]bool{"11": true, "32": true}

// WriteInvoicesPDF (abstraction) creates PDF from data
func WriteInvoicesPDF(invoices []*BookedInvoice) ([]byte, error) {
	ac := &accounting.Accounting{Precision: 2, Thousand: ".", Decimal: ","}
	// Init DB
	db, err := storage.InitFirebaseDB()
	// Gather PDF values in struct
	pdfLines := handleValues(db, invoices)
	// calculate total values
	totals := calcTotalVals(pdfLines)
	// Write new PDF
	pdf := newPDF()
	pdf = writeHeader(pdf, []string{"Inv.#", "Date", "Customer", "Country", "Period", "Max seller cut", "Min. Byrd cut", "VAT", "Total price (DKK)"})
	pdf = writeBody(pdf, pdfLines, ac)
	pdf = writeFooter(pdf, totals, ac)
	file, err := createPDF(pdf)
	if err != nil {
		return nil, err
	}
	fmt.Println("Created PDF")
	return file, nil
}

func newPDF() *gofpdf.Fpdf {
	pdf := gofpdf.New("L", "mm", "Letter", "")
	pdf.AddPage()
	pdf.SetFont("Times", "B", 16)
	pdf.Cell(40, 10, "Media usage report")
	pdf.Ln(10)
	pdf.SetFont("Times", "", 10)
	pdf.Cell(40, 10, "Generated: "+time.Now().Format("Mon Jan 2, 2006"))
	bImg := storage.GetAWSSecrets("byrd.png")
	r := bytes.NewReader(bImg)
	opts := gofpdf.ImageOptions{
		ImageType: "PNG",
		ReadDpi:   true,
	}
	pdf.RegisterImageOptionsReader("byrd.png", opts, r)
	pdf.ImageOptions("byrd.png", 225, 5, 25, 25, false, opts, 0, "")
	pdf.Ln(14)
	return pdf
}

func writeHeader(pdf *gofpdf.Fpdf, hdr []string) *gofpdf.Fpdf {
	pdf.SetFont("Times", "B", 10)
	pdf.SetFillColor(240, 240, 240)
	for i, str := range hdr {
		pdf.CellFormat(ftrHdrSizes[i], 7, str, "1", 0, "", true, 0, "")
	}
	pdf.Ln(-1)
	return pdf
}

// Special invoices to not be booked
func nilBookedInv() map[int]bool {
	return map[int]bool{128: true, 129: true, 131: true, 132: true}
}

func handleValues(db *storage.DBInstance, invoices []*BookedInvoice) []*PDFLine {
	pdfLines := []*PDFLine{}
	for _, invoice := range invoices {
		for _, line := range invoice.Lines {
			product, err := storage.GetSubscriptionProducts(db, line.Product.ProductNumber)
			if err != nil {
				log.Panicf("Didnt get products from FB: %s", err)
			}
			product.Credits = line.handleIfPAYGCredits(product)
			product = product.IsYearlyProduct()
			line = line.isEuroAmount(invoice)
			// Adding to PDF values slice
			pdfLines = line.addToLine(pdfLines, invoice, product)
		}
	}
	return pdfLines
}

// Function inside invoice loop
func (l *Lines) addToLine(pdfLines []*PDFLine, i *BookedInvoice, p *storage.SubscriptionProduct) []*PDFLine {
	switch p.IsNonPlatform() {
	case false:
		pdfLine := PDFLine{
			InvoiceNum:   i.BookedInvoiceNumber,
			Recipient:    i.Recipient,
			Date:         i.Date,
			MaxSellerCut: l.maxSellerCut(p),
			MinByrdInc:   l.minByrdInc(p),
			Period:       setPeriod(p.Period),
			VAT:          i.applyTax(l),
			NetAmount:    l.TotalNetAmount,
		}
		pdfLines = append(pdfLines, &pdfLine)

	case true:
		fmt.Printf("Single sale inv#: %v!\n", i.BookedInvoiceNumber)
		pdfLine := PDFLine{
			InvoiceNum:   i.BookedInvoiceNumber,
			Recipient:    i.Recipient,
			Date:         i.Date,
			MaxSellerCut: p.GetSellerCut(),
			MinByrdInc:   i.NetAmount - p.GetSellerCut(),
			Period:       "ONE-TIME",
			VAT:          i.VatAmount,
			NetAmount:    i.NetAmount,
		}

		pdfLines = append(pdfLines, &pdfLine)
		fmt.Printf("Credits: %v. VAT: %v. Period: %s \n", p.Credits, pdfLine.VAT, pdfLine.Period)
	}
	return pdfLines
}

func calcTotalVals(vals []*PDFLine) *TotalVals {
	totalVals := &TotalVals{}
	for _, v := range vals {
		totalVals.TotalByrdInc += v.MinByrdInc
		totalVals.TotalNetAmount += v.NetAmount
		totalVals.TotalSellerCut += v.MaxSellerCut
		totalVals.TotalVAT += v.VAT
	}
	return totalVals
}

func writeBody(pdf *gofpdf.Fpdf, pdfLines []*PDFLine, ac *accounting.Accounting) *gofpdf.Fpdf {
	pdf.SetFont("Times", "", 10)

	pdf.SetFillColor(240, 240, 240)
	// {20, 30, 50, 20, 30, 30, 20, 40}
	for _, line := range pdfLines {
		pdf.CellFormat(20, 8, strconv.Itoa(line.InvoiceNum), "1", 0, "", true, 0, "")
		pdf.CellFormat(20, 8, line.Date, "1", 0, "", true, 0, "")
		pdf.CellFormat(50, 8, line.Recipient.Name, "1", 0, "", true, 0, "")
		pdf.CellFormat(20, 8, line.Recipient.Country, "1", 0, "", true, 0, "")
		pdf.CellFormat(20, 8, line.Period, "1", 0, "", true, 0, "")
		pdf.CellFormat(30, 8, ac.FormatMoneyFloat64(line.MaxSellerCut), "1", 0, "", true, 0, "")
		pdf.CellFormat(30, 8, ac.FormatMoneyFloat64(line.MinByrdInc), "1", 0, "", true, 0, "")
		pdf.CellFormat(30, 8, ac.FormatMoneyFloat64(line.VAT), "1", 0, "", true, 0, "")
		pdf.CellFormat(30, 8, ac.FormatMoneyFloat64(line.NetAmount+line.VAT), "1", 0, "", true, 0, "")
		pdf.Ln(6)
		fmt.Printf("Wrote invoice#: %v to customer: %s with amount: %v\n", line.InvoiceNum, line.Recipient.Name, line.NetAmount)
	}
	return pdf
}

func writeFooter(pdf *gofpdf.Fpdf, ftr *TotalVals, ac *accounting.Accounting) *gofpdf.Fpdf {
	pdf.SetFont("Times", "B", 10)
	pdf.SetFillColor(240, 240, 240)
	pdf.Cell(20, 10, "Total amounts:")
	pdf.Cell(20, 10, "")
	pdf.Cell(50, 10, "")
	pdf.Cell(20, 10, "")
	pdf.Cell(20, 10, "")
	pdf.Cell(30, 10, ac.FormatMoneyFloat64(ftr.TotalSellerCut))
	pdf.Cell(30, 10, ac.FormatMoneyFloat64(ftr.TotalByrdInc))
	pdf.Cell(30, 10, ac.FormatMoneyFloat64(ftr.TotalVAT))
	pdf.Cell(30, 10, ac.FormatMoneyFloat64(ftr.TotalNetAmount+ftr.TotalVAT))
	pdf.Ln(-1)
	return pdf
}

func createPDF(pdf *gofpdf.Fpdf) ([]byte, error) {
	fmt.Println("Creating mem pdf")
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		if err := pdf.OutputAndClose(pw); err != nil {
			panic(err)
		}
	}()
	b, err := ioutil.ReadAll(pr)
	if err != nil {
		return nil, err
	}
	return b, nil

}

func (l *Lines) handleIfWrongLineNumber(i *BookedInvoice) *Lines {
	if l.SortKey != l.LineNumber {
		if l.LineNumber != productLineNumber {
			l.LineNumber = productLineNumber
		}
	}
	fmt.Printf("Fixed line: %v for product %s and invoice#: %v\n", l.LineNumber, l.Product, i.BookedInvoiceNumber)
	return l
}

func (l *Lines) handleIfPAYGCredits(p *storage.SubscriptionProduct) int {
	if l.Product.ProductNumber == productPAYG {
		credits := int(l.Quantity)
		fmt.Printf("Credit amount was calculated based on PAYG amount: %v\n", credits)
		return credits
	}
	return p.Credits
}

func (l *Lines) isEuroAmount(i *BookedInvoice) *Lines {
	if i.Currency == eur {
		l.TotalNetAmount *= euroToDKKPrice
	}
	return l
}

func (l *Lines) minByrdInc(p *storage.SubscriptionProduct) float64 {
	if p.Credits != unlimitedAmountCredits && l.TotalNetAmount > 0 {
		value := l.TotalNetAmount - math.Abs(l.maxSellerCut(p))
		if value < 0 {
			return 0
		}
		return value
	}
	return 0
}

func (l *Lines) maxSellerCut(p *storage.SubscriptionProduct) float64 {
	if p.Credits != unlimitedAmountCredits && l.TotalNetAmount > 0 {
		totalAmt := (photographerCut * euroToDKKPrice) * parseIntToFloat(p.Credits)
		return totalAmt
	}
	return 0
}

func setPeriod(p string) string {
	switch p {
	case month:
		return "MONTH"
	case year:
		return "YEAR"
	default:
		return "%"
	}
}

func (i *BookedInvoice) applyTax(l *Lines) float64 {
	if i.Recipient.Country == denmark || i.Recipient.Country == danmark {
		return l.VatAmount
	}
	return 0.00
}

func formatFloatToStr(n float64) string {
	return strconv.FormatFloat(n, 'f', 2, 64)
}

func parseIntToFloat(n int) float64 {
	return float64(n)
}
