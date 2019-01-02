package aws

import (
	"encoding/json"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/pkg/errors"

	"github.com/supergiant/analyze/builtin/plugins/sunsetting/cloudprovider"
	"github.com/supergiant/analyze/pkg/plugin/proto"
)

const region = "us-east-1"
const serviceCode = "AmazonEC2"
// filters
const neededProductFamily = "Compute Instance"
const neededOperatingSystem = "Linux"
const neededPreInstalledSw = "NA"

//var awsPartitions = map[string]string{
//	"ap-northeast-1": "Asia Pacific (Tokyo)",
//	"ap-northeast-2": "Asia Pacific (Seoul)",
//	"ap-south-1":     "Asia Pacific (Mumbai)",
//	"ap-southeast-1": "Asia Pacific (Singapore)",
//	"ap-southeast-2": "Asia Pacific (Sydney)",
//	"ca-central-1":   "Canada (Central)",
//	"eu-central-1":   "EU (Frankfurt)",
//	"eu-west-1":      "EU (Ireland)",
//	"eu-west-2":      "EU (London)",
//	"eu-west-3":      "EU (Paris)",
//	"sa-east-1":      "South America (Sao Paulo)",
//	"us-east-1":      "US East (N. Virginia)",
//	"us-east-2":      "US East (Ohio)",
//	"us-west-1":      "US West (N. California)",
//	"us-west-2":      "US West (Oregon)",
//}

type Client struct {
	//regionDescription string
	ec2Service        *ec2.EC2
	//pricingService    *pricing.Pricing
	logger logrus.FieldLogger
}

//NewClient creates aws client
func NewClient(clientConfig *proto.AwsConfig, logger logrus.FieldLogger) (*Client, error) {
	var region = clientConfig.GetRegion()
	var c = &Client{
		//regionDescription: awsPartitions[region],
		logger:logger,
	}
	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		return nil, errors.Wrap(err, "unable to load AWS SDK config")
	}

	// TODO bug in sdk?
	//cfg.Region = "us-east-1"
	//c.pricingService = pricing.New(cfg)

	// set correct region for ec2 service
	cfg.Region = region
	c.ec2Service = ec2.New(cfg)

	return c, nil
}

func (c *Client) GetPrices() (map[string][]cloudprovider.ProductPrice, error) {
	var computeInstancesPrices = make(map[string][]cloudprovider.ProductPrice, 0)

	var offeringsURI = "https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/" + serviceCode + "/current/" + region + "/index.json"
	offeringsRaw, err :=  http.Get(offeringsURI)
	if err != nil {
		return nil, errors.Wrap(err, "can't download prices")
	}
	defer offeringsRaw.Body.Close()
	if offeringsRaw.StatusCode != http.StatusOK {
		return nil, errors.Errorf("can't download prices, server returned %s", offeringsRaw.Status)
	}

	// file size is about 40 - 50 megabytes
	offeringBytes, err := ioutil.ReadAll(offeringsRaw.Body)

	type prices struct {
		Products map[string]struct {
			Sku           string `json:"sku"`
			ProductFamily string `json:"productFamily"`
			Attributes    struct {
				InstanceType string `json:"instanceType"`
				Memory       string `json:"memory"`
				Vcpu         string `json:"vcpu"`
				Usagetype    string `json:"usagetype"`
				Tenancy      string `json:"tenancy"`
				OperatingSystem      string `json:"operatingSystem"`
				PreInstalledSw      string `json:"preInstalledSw"`
			} `json:"attributes"`
		} `json:"products"`
		Terms struct {
			// OnDemand map[Sku]map[Sku.offerTermCode]struct{...
			OnDemand map[string]map[string]struct {
				PriceDimensions map[string]struct {
					Unit         string `json:"unit"`
					PricePerUnit struct {
						USDRate string `json:"USD"`
					} `json:"pricePerUnit"`
				} `json:"priceDimensions"`
			} `json:"OnDemand"`
		} `json:"terms"`
	}

	offerings := &prices{}

	err = json.Unmarshal(offeringBytes, offerings)
	if err != nil {
		return nil, errors.Wrap(err, "can't unmarshal prices")
	}

	for productSku, product := range offerings.Products {
		if product.ProductFamily != neededProductFamily ||
			product.Attributes.OperatingSystem != neededOperatingSystem ||
			product.Attributes.PreInstalledSw != neededPreInstalledSw {
			continue
		}

		var newPriceItem = cloudprovider.ProductPrice{
			InstanceType: product.Attributes.InstanceType,
			Memory:       product.Attributes.Memory,
			Vcpu:         product.Attributes.Vcpu,
			Unit:         "",
			ValuePerUnit: "",
			Currency:     "USD",
			UsageType:    product.Attributes.Usagetype,
			Tenancy:      product.Attributes.Tenancy,
		}

		for _, price := range offerings.Terms.OnDemand[productSku] {
			for _, priceDimension := range  price.PriceDimensions {
				newPriceItem.Unit = priceDimension.Unit
				newPriceItem.ValuePerUnit=  priceDimension.PricePerUnit.USDRate
				break
			}
		}

		_, exists := computeInstancesPrices[product.Attributes.InstanceType]
		if !exists {
			computeInstancesPrices[product.Attributes.InstanceType] = make([]cloudprovider.ProductPrice, 0, 0)
		}
		computeInstancesPrices[product.Attributes.InstanceType] = append(computeInstancesPrices[product.Attributes.InstanceType], newPriceItem)
	}

	return computeInstancesPrices, nil
}

//func (c *Client) GetPrices() (map[string][]cloudprovider.ProductPrice, error) {
//	var computeInstancesPrices = make(map[string][]cloudprovider.ProductPrice, 0)
//
//	productsInput := &pricing.GetProductsInput{
//		Filters: []pricing.Filter{
//			{
//				Field: aws.String("ServiceCode"),
//				Type:  pricing.FilterTypeTermMatch,
//				Value: aws.String("AmazonEC2"),
//			},
//			{
//				Field: aws.String("productFamily"),
//				Type:  pricing.FilterTypeTermMatch,
//				Value: aws.String("Compute Instance"),
//			},
//			{
//				Field: aws.String("operatingSystem"),
//				Type:  pricing.FilterTypeTermMatch,
//				Value: aws.String("Linux"),
//			},
//			{
//				Field: aws.String("preInstalledSw"),
//				Type:  pricing.FilterTypeTermMatch,
//				Value: aws.String("NA"),
//			},
//			//TODO: FIRST PRIORITY FIX, to filter by usagetype "EC2: Running Hours"
//			//https://docs.aws.amazon.com/awsaccountbilling/latest/aboutv2/selectdim.html
//			//{
//			//	Field: aws.String("tenancy"),
//			//	Type:  pricing.FilterTypeTermMatch,
//			//	Value: aws.String("Shared"),
//			//},
//			{
//				Field: aws.String("location"),
//				Type:  pricing.FilterTypeTermMatch,
//				Value: aws.String(c.regionDescription), //TODO region to location??? bug, add PR to lib?
//			},
//		},
//		FormatVersion: aws.String("aws_v1"),
//		MaxResults:    aws.Int64(100),
//		ServiceCode:   aws.String("AmazonEC2"),
//	}
//
//	productsRequest := c.pricingService.GetProductsRequest(productsInput)
//
//	productsPager := productsRequest.Paginate()
//	for productsPager.Next() {
//		page := productsPager.CurrentPage()
//
//		if page != nil {
//			for _, productItem := range page.PriceList {
//				var newPriceItem, err = getProduct(productItem)
//				if err != nil {
//					// it is not critical need just to log it
//					//return nil, err
//				}
//				_, exists := computeInstancesPrices[newPriceItem.InstanceType]
//				if !exists {
//					computeInstancesPrices[newPriceItem.InstanceType] = make([]cloudprovider.ProductPrice, 0, 0)
//				}
//				computeInstancesPrices[newPriceItem.InstanceType] = append(computeInstancesPrices[newPriceItem.InstanceType], *newPriceItem)
//			}
//		}
//	}
//
//	if err := productsPager.Err(); err != nil {
//		return nil, errors.Wrap(err, "failed to describe products")
//	}
//
//	fmt.Printf("found product prices: %v\n", len(computeInstancesPrices))
//	return computeInstancesPrices, nil
//}
//
//// TODO add checks and return error
//func getProduct(productItem aws.JSONValue) (*cloudprovider.ProductPrice, error) {
//	var result = &cloudprovider.ProductPrice{}
//	type prices struct {
//		Products map[string]struct 	{
//			Sku string `json:"sku"`
//			ProductFamily string `json:"productFamily"`
//			Attributes struct {
//				InstanceType string `json:"instanceType"`
//				Memory       string `json:"memory"`
//				Vcpu         string `json:"vcpu"`
//				Usagetype    string `json:"usagetype"`
//				Tenancy      string `json:"tenancy"`
//			} `json:"attributes"`
//		}`json:"products"`
//		Terms struct {
//			OnDemand map[string]struct {
//				Sku map[string] struct{
//					PriceDimensions map[string]struct {
//						Unit         string `json:"unit"`
//						PricePerUnit struct {
//							USDRate string `json:"USD"`
//						} `json:"pricePerUnit"`
//					} `json:"priceDimensions"`
//				}
//			} `json:"OnDemand"`
//		} `json:"terms"`
//	}
//	type productPrice struct {
//		Product struct {
//			Attributes struct {
//				InstanceType string `json:"instanceType"`
//				Memory       string `json:"memory"`
//				Vcpu         string `json:"vcpu"`
//				Usagetype    string `json:"usagetype"`
//				Tenancy      string `json:"tenancy"`
//			} `json:"attributes"`
//		} `json:"product"`
//		Terms struct {
//			OnDemand map[string]struct {
//				PriceDimensions map[string]struct {
//					Unit         string `json:"unit"`
//					PricePerUnit struct {
//						USDRate string `json:"USD"`
//					} `json:"pricePerUnit"`
//				} `json:"priceDimensions"`
//			} `json:"OnDemand"`
//		} `json:"terms"`
//	}
//
//	// oh boy, marshal again?
//	b, err := json.Marshal(productItem)
//	if err != nil {
//		return nil, err
//	}
//
//	var pp = &productPrice{}
//	err = json.Unmarshal(b, pp)
//	if err != nil {
//		return nil, err
//	}
//
//	result.InstanceType = pp.Product.Attributes.InstanceType
//	result.Memory = pp.Product.Attributes.Memory
//	result.Vcpu = pp.Product.Attributes.Vcpu
//	result.UsageType = pp.Product.Attributes.Usagetype
//	result.Tenancy = pp.Product.Attributes.Tenancy
//	if len(pp.Terms.OnDemand) < 1 {
//		return nil, errors.New("there is no OnDemand prices")
//	}
//
//	for _, term := range pp.Terms.OnDemand {
//		for _, onDemandTerm := range term.PriceDimensions {
//			result.Unit = onDemandTerm.Unit
//			result.ValuePerUnit = onDemandTerm.PricePerUnit.USDRate
//			result.Currency = "USD"
//		}
//
//	}
//
//	return result, nil
//}

func (c *Client) GetComputeInstances() (map[string]cloudprovider.ComputeInstance, error) {
	var instancesRequest = c.ec2Service.DescribeInstancesRequest(nil)
	var result = map[string]cloudprovider.ComputeInstance{}
	describeInstancesResponse, err := instancesRequest.Send()
	if err != nil {
		return nil, err
	}

	for _, instancesReservation := range describeInstancesResponse.Reservations {
		for _, i := range instancesReservation.Instances {
			if i.InstanceId == nil {
				continue
			}

			var instanceType, _ = i.InstanceType.MarshalValue()

			result[*i.InstanceId] = cloudprovider.ComputeInstance{
				InstanceID:   *i.InstanceId,
				InstanceType: instanceType,
			}
		}
	}

	return result, nil
}
