[![Maintained by Gruntwork.io](https://img.shields.io/badge/maintained%20by-gruntwork.io-%235849a6.svg)](https://gruntwork.io/?ref=repo_cloud_nuke)

# Cloud-Control-Nuke 

A cloud-control-backed fork of https://github.com/gruntwork-io/cloud-nuke

# Example commands to try

## Nuke several resources by type

```bash 
aws-vault exec <your-account-profile> --no-session \
  -- ./cloud-nuke aws \
  --region us-east-1 \
  --resource-type "AWS::IAM::Role" \
  --resource-type "AWS::EC2::FlowLog" \
  --resource-type "AWS::Logs::LogGroup" 
```

## List all currently supported resource types

```bash 

aws-vault exec <your-account-profile> --no-session \
  -- ./cloud-nuke aws \
  --region us-east-1 \
  --list-resource-types
```

You'll get back all the resource types that you can pass via one or more `--resource-type` flags: 

```
AWS::ACMPCA::Certificate
AWS::ACMPCA::CertificateAuthority
AWS::ACMPCA::CertificateAuthorityActivation
AWS::APS::RuleGroupsNamespace
AWS::APS::Workspace
AWS::AccessAnalyzer::Analyzer
AWS::Amplify::App
AWS::Amplify::Branch
AWS::Amplify::Domain
AWS::AmplifyUIBuilder::Component
AWS::AmplifyUIBuilder::Theme
AWS::ApiGateway::Account
AWS::ApiGateway::ApiKey
AWS::ApiGateway::Stage
AWS::ApiGateway::UsagePlan
AWS::ApiGatewayV2::VpcLink
AWS::AppFlow::ConnectorProfile
... Truncated for brevity ...
```

## Results report 

At the end of a run you'll get a table displaying any available information about each resource found and whether or not it was successfully nuked:

```bash
Resource Identifier             | OperationStatus | StatusMessage | Error
AWSServiceRoleForSupport        | Not Available   | Not Available | waiter state transitioned to Failure
AWSServiceRoleForTrustedAdvisor | Not Available   | Not Available | waiter state transitioned to Failure
MyOtherTestRole                 | Not Available   | Not Available | waiter state transitioned to Failure
myOtherTestRoleAgain            | Not Available   | Not Available | waiter state transitioned to Failure
myTestRole                      | Not Available   | Not Available | waiter state transitioned to Failure
myTestRoleName                  | Not Available   | Not Available | waiter state transitioned to Failure
```

## Code-diff 

See the current changes here: https://github.com/gruntwork-io/cloud-nuke/compare/master...zackproser:cloud-nuke:master

The potential for simplification (and the possiblility of unlocking all current and future AWS cloudcontrol-compliant resources for free), appears significant. 
