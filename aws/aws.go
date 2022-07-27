package aws

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cloudformation_types "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	awsgo "github.com/aws/aws-sdk-go/aws"
	"github.com/gruntwork-io/cloud-nuke/config"
	"github.com/gruntwork-io/cloud-nuke/externalcreds"
	"github.com/gruntwork-io/cloud-nuke/logging"
	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/pterm/pterm"
)

// OptInNotRequiredRegions contains all regions that are enabled by default on new AWS accounts
// Beginning in Spring 2019, AWS requires new regions to be explicitly enabled
// See https://aws.amazon.com/blogs/security/setting-permissions-to-enable-accounts-for-upcoming-aws-regions/
var OptInNotRequiredRegions = []string{
	"eu-north-1",
	"ap-south-1",
	"eu-west-3",
	"eu-west-2",
	"eu-west-1",
	"ap-northeast-3",
	"ap-northeast-2",
	"ap-northeast-1",
	"sa-east-1",
	"ca-central-1",
	"ap-southeast-1",
	"ap-southeast-2",
	"eu-central-1",
	"us-east-1",
	"us-east-2",
	"us-west-1",
	"us-west-2",
}

// GovCloudRegions contains all of the U.S. GovCloud regions. In accounts with GovCloud enabled, these are the
// only available regions.
var GovCloudRegions = []string{
	"us-gov-east-1",
	"us-gov-west-1",
}

const (
	GlobalRegion string = "global"
	// us-east-1 is the region that is available in every account
	defaultRegion string = "us-east-1"
)

func newConfig(region string) (aws.Config, error) {
	return externalcreds.Get(region)
}

// Try a describe regions command with the most likely enabled regions
func retryDescribeRegions() (*ec2.DescribeRegionsOutput, error) {
	regionsToTry := append(OptInNotRequiredRegions, GovCloudRegions...)
	for _, region := range regionsToTry {
		config, loadConfigErr := newConfig(region)
		if loadConfigErr != nil {
			return nil, loadConfigErr
		}
		svc := ec2.NewFromConfig(config)
		regions, err := svc.DescribeRegions(context.Background(), &ec2.DescribeRegionsInput{})
		if err != nil {
			continue
		}
		return regions, nil
	}
	return nil, errors.WithStackTrace(fmt.Errorf("could not find any enabled regions"))
}

// GetEnabledRegions - Get all regions that are enabled (DescribeRegions excludes those not enabled by default)
func GetEnabledRegions() ([]string, error) {
	var regionNames []string

	// We don't want to depend on a default region being set, so instead we
	// will choose a region from the list of regions that are enabled by default
	// and use that to enumerate all enabled regions.
	// Corner case: user has intentionally disabled one or more regions that are
	// enabled by default. If that region is chosen, API calls will fail.
	// Therefore we retry until one of the regions works.
	regions, err := retryDescribeRegions()
	if err != nil {
		return nil, err
	}

	for _, region := range regions.Regions {
		regionNames = append(regionNames, awsgo.StringValue(region.RegionName))
	}

	return regionNames, nil
}

func getRandomRegion() (string, error) {
	return getRandomRegionWithExclusions([]string{})
}

// getRandomRegionWithExclusions - return random from enabled regions, excluding regions from the argument
func getRandomRegionWithExclusions(regionsToExclude []string) (string, error) {
	allRegions, err := GetEnabledRegions()
	if err != nil {
		return "", errors.WithStackTrace(err)
	}
	rand.Seed(time.Now().UnixNano())

	// exclude from "allRegions"
	exclusions := make(map[string]string)
	for _, region := range regionsToExclude {
		exclusions[region] = region
	}
	// filter regions
	var updatedRegions []string
	for _, region := range allRegions {
		_, excluded := exclusions[region]
		if !excluded {
			updatedRegions = append(updatedRegions, region)
		}
	}
	randIndex := rand.Intn(len(updatedRegions))
	logging.Logger.Infof("Random region chosen: %s", updatedRegions[randIndex])
	return updatedRegions[randIndex], nil
}

func split(identifiers []string, limit int) [][]string {
	if limit < 0 {
		limit = -1 * limit
	} else if limit == 0 {
		return [][]string{identifiers}
	}

	var chunk []string
	chunks := make([][]string, 0, len(identifiers)/limit+1)
	for len(identifiers) >= limit {
		chunk, identifiers = identifiers[:limit], identifiers[limit:]
		chunks = append(chunks, chunk)
	}
	if len(identifiers) > 0 {
		chunks = append(chunks, identifiers[:])
	}

	return chunks
}

// GetTargetRegions - Used enabled, selected and excluded regions to create a
// final list of valid regions
func GetTargetRegions(enabledRegions []string, selectedRegions []string, excludedRegions []string) ([]string, error) {
	if len(enabledRegions) == 0 {
		return nil, fmt.Errorf("Cannot have empty enabled regions")
	}

	// neither selectedRegions nor excludedRegions => select enabledRegions
	if len(selectedRegions) == 0 && len(excludedRegions) == 0 {
		return enabledRegions, nil
	}

	if len(selectedRegions) > 0 && len(excludedRegions) > 0 {
		return nil, fmt.Errorf("Cannot specify both selected and excluded regions")
	}

	var invalidRegions []string

	// Validate selectedRegions
	for _, selectedRegion := range selectedRegions {
		if !collections.ListContainsElement(enabledRegions, selectedRegion) {
			invalidRegions = append(invalidRegions, selectedRegion)
		}
	}
	if len(invalidRegions) > 0 {
		return nil, fmt.Errorf("Invalid values for region: [%s]", invalidRegions)
	}

	if len(selectedRegions) > 0 {
		return selectedRegions, nil
	}

	// Validate excludedRegions
	for _, excludedRegion := range excludedRegions {
		if !collections.ListContainsElement(enabledRegions, excludedRegion) {
			invalidRegions = append(invalidRegions, excludedRegion)
		}
	}
	if len(invalidRegions) > 0 {
		return nil, fmt.Errorf("Invalid values for exclude-region: [%s]", invalidRegions)
	}

	// Filter out excludedRegions from enabledRegions
	var targetRegions []string
	if len(excludedRegions) > 0 {
		for _, region := range enabledRegions {
			if !collections.ListContainsElement(excludedRegions, region) {
				targetRegions = append(targetRegions, region)
			}
		}
	}
	if len(targetRegions) == 0 {
		return nil, fmt.Errorf("Cannot exclude all regions: %s", excludedRegions)
	}
	return targetRegions, nil
}

// GetAllResources - Lists all aws resources
func GetAllResources(targetRegions []string, excludeAfter time.Time, resourceTypes []string, configObj config.Config) (*AwsAccountResources, error) {
	account := AwsAccountResources{
		Resources: make(map[string]AwsRegionResource),
	}

	count := 1
	totalRegions := len(targetRegions)

	for _, region := range targetRegions {
		// The "global" region case is handled outside this loop
		if region == GlobalRegion {
			continue
		}

		logging.Logger.Infof("Checking region [%d/%d]: %s", count, totalRegions, region)

		awsConfig, configLoadErr := newConfig(region)
		if configLoadErr != nil {
			return nil, configLoadErr
		}

		resourcesInRegion := AwsRegionResource{}

		svc := cloudcontrol.NewFromConfig(awsConfig)

		// TODO - move me to the right place
		/*resourcesToNuke, loadErr := LoadNukePlan()
		if loadErr != nil {
			return nil, loadErr
		}*/

		for _, resourceType := range resourceTypes {
			listInput := &cloudcontrol.ListResourcesInput{
				TypeName: aws.String(resourceType),
			}

			output, err := svc.ListResources(context.TODO(), listInput)
			if err != nil {
				fmt.Printf("Error listing resources: %+v\n", err)
			}

			resourceIdentifiers := []string{}

			for _, resourceDescription := range output.ResourceDescriptions {
				logging.Logger.Debugf("Found resource (%s) with properties: %+v\n", aws.ToString(resourceDescription.Identifier), aws.ToString(resourceDescription.Properties))
				resourceIdentifiers = append(resourceIdentifiers, aws.ToString(resourceDescription.Identifier))
			}

			awsResource := &AwsResource{
				TypeName:    resourceType,
				Identifiers: resourceIdentifiers,
			}

			resourcesInRegion.Resources = append(resourcesInRegion.Resources, awsResource)
		}

		if len(resourcesInRegion.Resources) > 0 {
			account.Resources[region] = resourcesInRegion
		}
		count++

	}

	return &account, nil
}

// ListResourceTypes - Returns list of resources which can be passed to --resource-type
func ListResourceTypes() []string {
	config, loadConfigErr := newConfig("us-east-1")
	if loadConfigErr != nil {
		logging.Logger.Errorf("Error loading aws config: %+v\n", loadConfigErr)
	}

	typeNameStrings := []string{}

	svc := cloudformation.NewFromConfig(config)
	listTypesInput := &cloudformation.ListTypesInput{
		DeprecatedStatus: cloudformation_types.DeprecatedStatusLive,
		Filters: &cloudformation_types.TypeFilters{
			Category: cloudformation_types.CategoryAwsTypes,
		},
		ProvisioningType: cloudformation_types.ProvisioningTypeFullyMutable,
		Visibility:       cloudformation_types.VisibilityPublic,
	}

	paginator := cloudformation.NewListTypesPaginator(svc, listTypesInput)

	pageNum := 0
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(context.TODO())
		if err != nil {
			log.Printf("error: %v", err)
			return typeNameStrings
		}
		for _, typeSummary := range output.TypeSummaries {
			typeNameStrings = append(typeNameStrings, aws.ToString(typeSummary.TypeName))
		}
		pageNum++
	}

	sort.Strings(typeNameStrings)
	return typeNameStrings
}

// IsValidResourceType - Checks if a resourceType is valid or not
func IsValidResourceType(resourceType string, allResourceTypes []string) bool {
	return collections.ListContainsElement(allResourceTypes, resourceType)
}

// IsNukeable - Checks if we should nuke a resource or not
func IsNukeable(resourceType string, resourceTypes []string) bool {
	if len(resourceTypes) == 0 ||
		collections.ListContainsElement(resourceTypes, "all") ||
		collections.ListContainsElement(resourceTypes, resourceType) {
		return true
	}
	return false
}

func nukeAllResourcesInRegion(account *AwsAccountResources, region string, config aws.Config) error {
	resourcesInRegion := account.Resources[region]

	tableData := make([][]string, 1)
	tableData = append(tableData, []string{"Resource", "Operation", "Status", "StatusMessage", "Error"})

	for _, resources := range resourcesInRegion.Resources {
		length := len(resources.ResourceIdentifiers())

		// Split api calls into batches
		logging.Logger.Infof("Terminating %d resources in batches", length)
		batches := split(resources.ResourceIdentifiers(), resources.MaxBatchSize())

		for i := 0; i < len(batches); i++ {
			batch := batches[i]
			returnedTableData, err := resources.Nuke(config, batch)
			if err != nil {
				// TODO: Figure out actual error type
				if strings.Contains(err.Error(), "RequestLimitExceeded") {
					logging.Logger.Info("Request limit reached. Waiting 1 minute before making new requests")
					time.Sleep(1 * time.Minute)
					continue
				}

				return errors.WithStackTrace(err)
			}

			for _, row := range returnedTableData {
				tableData = append(tableData, row)
			}

			if i != len(batches)-1 {
				logging.Logger.Info("Sleeping for 10 seconds before processing next batch...")
				time.Sleep(10 * time.Second)
			}
		}
	}

	// Print regional results
	if len(resourcesInRegion.Resources) > 0 {
		pterm.Println()

		renderSection(fmt.Sprintf("Region: %s", region))

		pterm.DefaultTable.
			WithHasHeader().
			WithData(tableData).
			Render()

		pterm.Println()

	}

	return nil
}

func renderSection(sectionTitle string) {
	pterm.DefaultSection.Style = pterm.NewStyle(pterm.FgLightCyan)
	pterm.DefaultSection.WithLevel(0).Println(sectionTitle)
}

// NukeAllResources - Nukes all aws resources
func NukeAllResources(account *AwsAccountResources, regions []string) error {
	for _, region := range regions {
		// region that will be used to create a session
		targetRegion := region

		// As there is no actual region named global we have to pick a valid one just to create the session
		if region == GlobalRegion {
			targetRegion = defaultRegion
		}

		config, err := newConfig(targetRegion)
		if err != nil {
			return errors.WithStackTrace(err)
		}

		err = nukeAllResourcesInRegion(account, region, config)

		if err != nil {
			return errors.WithStackTrace(err)
		}

	}

	return nil
}
