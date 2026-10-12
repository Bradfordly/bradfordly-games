#!/usr/bin/env node
import * as cdk from 'aws-cdk-lib/core';
import { GamesStack } from '../lib/games-stack';

const app = new cdk.App();

// Account and region come from the environment (12-factor), never hardcoded.
// The CDK CLI sets CDK_DEFAULT_* from the active AWS profile/credentials.
new GamesStack(app, 'BradfordlyGames', {
  env: {
    account: process.env.CDK_DEFAULT_ACCOUNT,
    region: process.env.CDK_DEFAULT_REGION,
  },
});
