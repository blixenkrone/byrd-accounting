package storage

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/api/option"

	"firebase.google.com/go/db"

	firebase "firebase.google.com/go"
)

// SubscriptionProduct is the economics invoice ID
type SubscriptionProduct struct {
	Credits       int     `json:"credits"` /* Important */
	FBID          string  `json:"id"`
	Period        string  `json:"period"`
	PhotoCut      float64 `json:"photoCut"`
	TotalAmount   float64 `json:"totalAmount"`
	ProductNumber string  // comes from the DB object name
}

// DBInstance -
type DBInstance struct {
	Client  *db.Client
	Context context.Context
}

// Nonplatform If sale != web app sale
const Nonplatform = "nonplatform"

// NonPlatformSale If the sale is outside byrd app
type NonPlatformSale interface {
	IsNonPlatform() bool
	SellerCut() float64
}

// InitFirebaseDB SE
func InitFirebaseDB() (*DBInstance, error) {
	ctx := context.Background()
	config := &firebase.Config{
		DatabaseURL: os.Getenv("FB_DATABASE_URL"),
	}
	jsonPath := "fb-" + os.Getenv("ENV") + ".json"
	opt := option.WithCredentialsJSON(GetAWSSecrets(jsonPath))
	app, err := firebase.NewApp(ctx, config, opt)
	if err != nil {
		panic(err)
	}
	client, err := app.Database(ctx)
	if err != nil {
		return nil, err
	}

	return &DBInstance{
		Client:  client,
		Context: ctx,
	}, nil
}

// GetSubscriptionProducts - this guy
func GetSubscriptionProducts(db *DBInstance, productNumber string) (*SubscriptionProduct, error) {
	path := os.Getenv("ENV") + "/subscriptionProducts/" + productNumber
	product := SubscriptionProduct{}
	fmt.Printf("Path: %s\n", path)
	ref := db.Client.NewRef(path)
	if err := ref.Get(db.Context, &product); err != nil {
		return nil, err
	}
	product.ProductNumber = productNumber
	return &product, nil
}

// GetSellerCut returns the cut for the seller ourside byrd app
func (p *SubscriptionProduct) GetSellerCut() float64 {
	if ok := p.IsNonPlatform(); ok != false {
		return p.PhotoCut
	}
	return 0
}

// IsNonPlatform is the sale outside web app?
func (p *SubscriptionProduct) IsNonPlatform() bool {
	if p.FBID == Nonplatform {
		return true
	}
	return false
}

// IsYearlyProduct if the product is yearly
func (p SubscriptionProduct) IsYearlyProduct() *SubscriptionProduct {
	if p.Period == "year" {
		p.Credits *= 12
	}
	return &p
}
