package aws

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/gruntwork-io/cloud-nuke/logging"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/hashicorp/go-multierror"
	"github.com/pterm/pterm"
	"gopkg.in/yaml.v2"
)

const AwsResourceExclusionTagKey = "cloud-nuke-excluded"

type AwsAccountResources struct {
	Resources map[string]AwsRegionResource
}

type ResourceTypeString string

func (r ResourceTypeString) String() string {
	return string(r)
}

type ResourcesToNuke struct {
	Targets []ResourceTypeString `yaml:"ResourcesToNuke"`
}

func (a *AwsAccountResources) GetRegion(region string) AwsRegionResource {
	if val, ok := a.Resources[region]; ok {
		return val
	}
	return AwsRegionResource{}
}

func LoadNukePlan() (*ResourcesToNuke, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	resourcesToNuke := &ResourcesToNuke{}
	nukePlanPath := filepath.Join(cwd, "nuke-plan.yml")
	f, openErr := os.Open(nukePlanPath)
	if openErr != nil {
		return nil, openErr
	}

	b, readErr := ioutil.ReadAll(f)
	if readErr != nil {
		return nil, readErr
	}

	unmarshalErr := yaml.Unmarshal(b, resourcesToNuke)
	if unmarshalErr != nil {
		return nil, unmarshalErr
	}

	return resourcesToNuke, nil
}

// MapResourceNameToIdentifiers converts a slice of Resources to a map of resource types to their found identifiers
// For example: ["ec2"] = ["i-0b22a22eec53b9321", "i-0e22a22yec53b9456"]
func (arr AwsRegionResource) MapResourceNameToIdentifiers() map[string][]string {
	// Initialize map of resource name to identifier, e.g., ["ec2"] = "i-0b22a22eec53b9321"
	m := make(map[string][]string)
	for _, resource := range arr.Resources {
		if len(resource.ResourceIdentifiers()) > 0 {
			for _, id := range resource.ResourceIdentifiers() {
				m[resource.ResourceName()] = append(m[resource.ResourceName()], id)
			}
		}
	}
	return m
}

// CountOfResourceType is a convenience method that returns the number of the supplied resource type found in the AwsRegionResource
func (arr AwsRegionResource) CountOfResourceType(resourceType string) int {
	idMap := arr.MapResourceNameToIdentifiers()
	resourceType = strings.ToLower(resourceType)
	if val, ok := idMap[resourceType]; ok {
		return len(val)
	}
	return 0
}

// ResourceTypePresent is a convenience method that returns true, if the given resource is found in the AwsRegionResource, or false if it is not
func (arr AwsRegionResource) ResourceTypePresent(resourceType string) bool {
	return arr.CountOfResourceType(resourceType) > 0
}

// IdentifiersForResourceType is a convenience method that returns the list of resource identifiers for a given resource type, if available
func (arr AwsRegionResource) IdentifiersForResourceType(resourceType string) []string {
	idMap := arr.MapResourceNameToIdentifiers()
	resourceType = strings.ToLower(resourceType)
	if val, ok := idMap[resourceType]; ok {
		return val
	}
	return []string{}
}

type AwsResource struct {
	TypeName    string
	Identifiers []string
}

func (a AwsResource) ResourceName() string {
	return a.TypeName
}

func (a AwsResource) ResourceIdentifiers() []string {
	return a.Identifiers
}

func (a AwsResource) MaxBatchSize() int {
	return 50
}

type AwsResourceResult struct {
	Identifier      string
	OperationStatus string
	StatusMessage   string
	Error           error
}

func (a AwsResource) Nuke(config aws.Config, identifiers []string) error {
	svc := cloudcontrol.NewFromConfig(config)

	if len(identifiers) > a.MaxBatchSize() {
		logging.Logger.Errorf("Nuking too many resources at once (%d): halting to avoid hitting AWS API rate limiting", len(identifiers))
		return TooManyResourcesTargetedErr{numTargets: len(identifiers)}
	}

	resultsMap := make(map[string]AwsResourceResult)

	logging.Logger.Infof("Nuking resource type (%s) in region (%s)", a.TypeName, config.Region)

	wg := new(sync.WaitGroup)
	wg.Add(len(identifiers))
	resultChans := make([]chan AwsResourceResult, len(identifiers))
	for i, identifier := range identifiers {
		resultChans[i] = make(chan AwsResourceResult, 1)
		go nukeAsync(wg, resultChans[i], svc, a.TypeName, identifier)
	}
	wg.Wait()

	var allErrs *multierror.Error
	for _, resultChan := range resultChans {
		result := <-resultChan
		// Update resultsMap with an entry for the Identifier and its result (error or nil)
		resultsMap[result.Identifier] = result
		if result.Error != nil {
			allErrs = multierror.Append(allErrs, result.Error)
		}
	}

	tableData := pterm.TableData{
		{"Resource Identifier", "OperationStatus", "StatusMessage", "Error"},
	}

	// Display results table
	for identifier, result := range resultsMap {
		tableData = append(tableData, []string{identifier, result.OperationStatus, result.StatusMessage, result.Error.Error()})
	}

	pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()

	pterm.Println()

	finalErr := allErrs.ErrorOrNil()
	if finalErr != nil {
		return errors.WithStackTrace(finalErr)
	}

	return nil
}

func nukeAsync(wg *sync.WaitGroup, resultChan chan AwsResourceResult, svc *cloudcontrol.Client, typeName, identifier string) {
	defer wg.Done()

	awsResourceResult := AwsResourceResult{
		Identifier: identifier,
		Error:      nil,
	}

	logging.Logger.Infof("Nuking resource type: %s with identifier: %s", typeName, identifier)

	deleteInput := &cloudcontrol.DeleteResourceInput{
		TypeName:   aws.String(typeName),
		Identifier: aws.String(identifier),
	}

	deleteOutput, deleteErr := svc.DeleteResource(context.Background(), deleteInput)
	if deleteErr != nil {
		awsResourceResult.Error = deleteErr

		resultChan <- awsResourceResult
		return
	}

	requestToken := deleteOutput.ProgressEvent.RequestToken

	waiter := cloudcontrol.NewResourceRequestSuccessWaiter(svc)

	waitParams := &cloudcontrol.GetResourceRequestStatusInput{
		RequestToken: requestToken,
	}

	// TODO - make this configurable
	maxWaitDur := time.Minute * 10

	logging.Logger.Infof("Waiting on deletion of resource type: %s with identifier: %s", typeName, identifier)

	statusOutput, waitErr := waiter.WaitForOutput(context.TODO(), waitParams, maxWaitDur)

	if statusOutput != nil {

		logging.Logger.Infof("DEBUG: statusOutput: %+v\n", statusOutput)
		logging.Logger.Infof("DEBUG: statusOutput.ProgressEvent: %+v\n", statusOutput.ProgressEvent)

		awsResourceResult.OperationStatus = string(statusOutput.ProgressEvent.OperationStatus)
		awsResourceResult.StatusMessage = string(aws.ToString(statusOutput.ProgressEvent.StatusMessage))
	} else {
		// Backfill statusOutput with user-friendly status message
		defaultMsg := "Not Available"

		awsResourceResult.OperationStatus = defaultMsg
		awsResourceResult.StatusMessage = defaultMsg
	}
	awsResourceResult.Error = waitErr
	resultChan <- awsResourceResult
}

type AwsResources interface {
	TypeName() string
	ResourceIdentifiers() []string
	Nuke(config aws.Config, identifiers []string) error
}

type AwsRegionResource struct {
	Resources []*AwsResource
}

// Query is a struct that represents the desired parameters for scanning resources within a given account
type Query struct {
	Regions              []string
	ExcludeRegions       []string
	ResourceTypes        []string
	ExcludeResourceTypes []string
	ExcludeAfter         time.Time
}

// NewQuery configures and returns a Query struct that can be passed into the InspectResources method
func NewQuery(regions, excludeRegions, resourceTypes, excludeResourceTypes []string, excludeAfter time.Time) (*Query, error) {
	q := &Query{
		Regions:              regions,
		ExcludeRegions:       excludeRegions,
		ResourceTypes:        resourceTypes,
		ExcludeResourceTypes: excludeResourceTypes,
		ExcludeAfter:         excludeAfter,
	}

	validationErr := q.Validate()

	if validationErr != nil {
		return q, validationErr
	}

	return q, nil
}

// Validate ensures the configured values for a Query are valid, returning an error if there are
// any invalid params, or nil if the Query is valid
func (q *Query) Validate() error {
	resourceTypes, err := HandleResourceTypeSelections(q.ResourceTypes, q.ExcludeResourceTypes)
	if err != nil {
		return err
	}

	q.ResourceTypes = resourceTypes

	regions, err := GetEnabledRegions()
	if err != nil {
		return CouldNotDetermineEnabledRegionsError{Underlying: err}
	}

	// global is a fake region, used to represent global resources
	regions = append(regions, GlobalRegion)

	targetRegions, err := GetTargetRegions(regions, q.Regions, q.ExcludeRegions)
	if err != nil {
		return CouldNotSelectRegionError{Underlying: err}
	}

	q.Regions = targetRegions

	return nil
}

// custom errors

type TooManyResourcesTargetedErr struct {
	numTargets int
}

func (err TooManyResourcesTargetedErr) Error() string {
	return fmt.Sprintf("You have selected too many resources (%d) to nuke at once. Halting to avoid hitting AWS API rate limits", err.numTargets)
}

type InvalidResourceTypesSuppliedError struct {
	InvalidTypes []string
}

func (err InvalidResourceTypesSuppliedError) Error() string {
	return fmt.Sprintf("Invalid resourceTypes %s specified: %s", err.InvalidTypes, "Try --list-resource-types to get a list of valid resource types.")
}

type ResourceTypeAndExcludeFlagsBothPassedError struct{}

func (err ResourceTypeAndExcludeFlagsBothPassedError) Error() string {
	return "You can not specify both --resource-type and --exclude-resource-type"
}

type InvalidTimeStringPassedError struct {
	Entry      string
	Underlying error
}

func (err InvalidTimeStringPassedError) Error() string {
	return fmt.Sprintf("Could not parse %s as a valid time duration. Underlying error: %s", err.Entry, err.Underlying)
}

type QueryCreationError struct {
	Underlying error
}

func (err QueryCreationError) Error() string {
	return fmt.Sprintf("Error forming a cloud-nuke Query with supplied parameters. Original error: %v", err.Underlying)
}

type ResourceInspectionError struct {
	Underlying error
}

func (err ResourceInspectionError) Error() string {
	return fmt.Sprintf("Error encountered when querying for account resources. Original error: %v", err.Underlying)
}

type CouldNotSelectRegionError struct {
	Underlying error
}

func (err CouldNotSelectRegionError) Error() string {
	return fmt.Sprintf("Unable to determine target region set. Please double check your combination of target and excluded regions. Original error: %v", err.Underlying)
}

type CouldNotDetermineEnabledRegionsError struct {
	Underlying error
}

func (err CouldNotDetermineEnabledRegionsError) Error() string {
	return fmt.Sprintf("Unable to determine enabled regions in target account. Original error: %v", err.Underlying)
}
