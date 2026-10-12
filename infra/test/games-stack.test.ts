import * as cdk from 'aws-cdk-lib/core';
import { Template } from 'aws-cdk-lib/assertions';
import { GamesStack } from '../lib/games-stack';
import { context } from '../cdk.json';

test('stack synthesizes with no resources', () => {
  // Match `cdk synth`: same feature flags, and CDKMetadata is emitted.
  const app = new cdk.App({ context, analyticsReporting: true });
  const stack = new GamesStack(app, 'TestStack');

  const resources = Template.fromStack(stack).toJSON().Resources ?? {};
  const nonMetadata = Object.values(resources).filter(
    (r: any) => r.Type !== 'AWS::CDK::Metadata',
  );

  expect(nonMetadata).toHaveLength(0);
});
