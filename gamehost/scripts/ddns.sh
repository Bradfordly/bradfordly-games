#!/bin/bash -xe

# this script dynamically sets the route53 record for the gamehost ec2 instance when it changes
# ec2 instance iam role must have access to update route53

# read ec2 metadata to determine region and public hostname
region=$(curl -sL http://169.254.169.254/latest/meta-data/placement/region)
ec2Host=$(curl -sL http://169.254.169.254/latest/meta-data/public-hostname)

# read from ssm parameter store to get route53 hosted zone
r53HostedZone=$(aws ssm get-parameter --name "coreHostedZone" --region us-east-1 | jq -r '.Parameter.Value')

# create route53 record object with the new ec2 hostname
record=$(jq -n \
  --arg r53hz "${r53HostedZone}" \
  --arg ec2h "${ec2Host}" \
  '{
  "Comment": "gamehost dynamic dns service",
  "Changes": [
    {
      "Action": "UPSERT",
      "ResourceRecordSet": {
        "Name": "gamehost.bradfordly.com",
        "Type": "CNAME",
        "TTL": 60,
        "ResourceRecords": [
          {
            "Value": $ec2h
          }
        ]
      }
    }
  ]
}'
)
echo $record > record.json

# update route53 with the new record
aws route53 change-resource-record-sets --hosted-zone-id $r53HostedZone --change-batch file://record.json --region $region
