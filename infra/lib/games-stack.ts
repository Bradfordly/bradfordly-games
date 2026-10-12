import * as cdk from 'aws-cdk-lib/core';
import { Construct } from 'constructs';

// Intentionally empty until ADR-0002 (#3) settles the hosting model.
export class GamesStack extends cdk.Stack {
  constructor(scope: Construct, id: string, props?: cdk.StackProps) {
    super(scope, id, props);
  }
}
