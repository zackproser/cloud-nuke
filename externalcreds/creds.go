package externalcreds

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

var externalConfig *aws.Config

func Set(opts *aws.Config) {
	externalConfig = opts
}

func Get(region string) (aws.Config, error) {
	// optsFuncs := []*config.LoadOptionsFunc{}

	/* TODO
	if externalConfig != nil {
		loadOptions.Credentials = externalConfig.Credentials
	}*/

	awsConfig, loadConfigErr := config.LoadDefaultConfig(
		context.TODO(),
		config.WithRegion(region),
	)
	if loadConfigErr != nil {
		return aws.Config{}, loadConfigErr
	}
	return awsConfig, nil
}
