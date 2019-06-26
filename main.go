package main

import (
	"fmt"
	"log"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/blixenkrone/byrd-accounting/invoices"
	"github.com/blixenkrone/byrd-accounting/slack"
	"github.com/blixenkrone/byrd-accounting/storage"
	"github.com/joho/godotenv"
)

/**
 * TODO: Faktura #135. Hvis unlimited plan => max seller cut er = subtotal for subscription (ex. moms)
 * *: Variabel unlimited plan => hvis unlimited er en årlig aftale fremgår prisen heraf, så ignorer PERIOD.
 * ! Sig det til Morten ^
 * *: Min. Byrd cut bliver 0 nogle gange (se faktura 151)
 * ! Sig til morten at faktura 151 får negativt beløb fordi det er noteret som en årsaftale (selvom det er en måned)
 * *: I stedet for customer navn så er det kunde nr.("customerNumber": 1105 i economoics)
 * *: Hvis produktnummer 25, indsæt subtotal beløbet i Min. Byrd cut colonnen (eksisterende med moms og total price).
 * *: MOMS skal beregnes i EUR hvis alle priser er i EURO (faktura nummer#: 159)

 * ?: Hvis kreditnota, træk alle linjebeløb fra total priserne (i alle rækkerne).
 */

func init() {
	if err := loadEnvironment(); err != nil {
		log.Printf("Error with env: %s", err)
	}
}

func main() {
	// Makefile lambdazip
	lambda.Start(HandleRequest)
	// HandleRequest() // 	testing:
}

// HandleRequest -
func HandleRequest() {
	/*CUSTOM DATES*/
	// dates := &invoices.DateRange{
	// 	From: "2019-05-01",
	// 	To:   "2019-05-31",
	// }
	// dates.Query = "date$gte:" + dates.From + "$and:date$lte:" + dates.To
	/*CUSTOM DATES*/

	dates := invoices.SetDateRange()

	file := CreateInvoice(dates)
	dirName, err := StoreOnAWS(file, dates)
	if err != nil {
		fmt.Printf("couldt upload to server: %s", err)
	}
	if err := NotifyOnSlack(dates, dirName); err != nil {
		fmt.Printf("Slack failed: %s", err)
	}
}

// CreateInvoice creates the initial PDF in memory
func CreateInvoice(d *invoices.DateRange) []byte {
	file, err := invoices.InitInvoiceOutput(d)
	if err != nil {
		fmt.Printf("Error on invoice output: %s", err)
	}
	return file
}

// StoreOnAWS Store the PDF on AWS
func StoreOnAWS(file []byte, d *invoices.DateRange) (string, error) {
	// Upload Mem PDF to S3
	dirName, err := storage.NewUpload(file, d.From)
	if err != nil {
		return "", err
	}
	return dirName, nil
}

// NotifyOnSlack notifies on slack upon new PDF
func NotifyOnSlack(dates *invoices.DateRange, dirName string) error {
	msg := &slack.MsgBuilder{
		TitleLink: "https://s3.console.aws.amazon.com/s3/buckets/byrd-accounting" + dirName,
		Text:      "New numbers for media subscriptions available as PDF!",
		Pretext:   "Click the link below to access it.",
		Period:    dates.From + "-" + dates.To,
		Color:     "#00711D",
		Footer:    "This is an auto-msg. Don't message me.",
	}
	if err := slack.NotifyPDFCreation(msg); err != nil {
		return err
	}
	return nil
}

func loadEnvironment() error {
	if err := godotenv.Load(); err != nil {
		return err
	}
	return nil
}
