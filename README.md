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

## Code-diff 

See the current changes here: https://github.com/gruntwork-io/cloud-nuke/compare/master...zackproser:cloud-nuke:master

The potential for simplification (and the possiblility of unlocking all current and future AWS cloudcontrol-compliant resources for free), appears significant. 
