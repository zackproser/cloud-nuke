[![Maintained by Gruntwork.io](https://img.shields.io/badge/maintained%20by-gruntwork.io-%235849a6.svg)](https://gruntwork.io/?ref=repo_cloud_nuke)

# cloud-control-backed refactor

Currently working: 

- This simple config file: 

```yaml
ResourcesToNuke: 
  - "AWS::Logs::LogGroup"
  - "AWS::IAM::Role"

```

Results in the cloud-nuke binary in this repo being able to list the following resources in my test account:

```
❯ aws-vault exec nuketest --no-session -- ./cloud-nuke aws --region us-east-1
[cloud-nuke] INFO[2022-07-25T22:40:49-04:00] The following resource types will be nuked:
[cloud-nuke] INFO[2022-07-25T22:40:50-04:00] Retrieving active AWS resources in [us-east-1]
[cloud-nuke] INFO[2022-07-25T22:40:50-04:00] Checking region [1/1]: us-east-1
Found resource (testy) of type: {"LogGroupName":"testy","Arn":"arn:aws:logs:us-east-1:297077893752:log-group:testy:*"}
Found resource (westside) of type: {"LogGroupName":"westside","Arn":"arn:aws:logs:us-east-1:297077893752:log-group:westside:*"}
Found resource (AWSServiceRoleForSupport) of type: {"RoleName":"AWSServiceRoleForSupport"}
Found resource (AWSServiceRoleForTrustedAdvisor) of type: {"RoleName":"AWSServiceRoleForTrustedAdvisor"}
Found resource (TestRole) of type: {"RoleName":"TestRole"}
```

## Code-diff 

See the current changes here: https://github.com/gruntwork-io/cloud-nuke/compare/master...zackproser:cloud-nuke:master

The potential for simplification (and the possiblility of unlocking all current and future AWS cloudcontrol-compliant resources for free), appears significant. 
