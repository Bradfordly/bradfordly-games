# infra

AWS CDK app (TypeScript) for bradfordly-games. See [ADR-0001](../docs/adr/0001-cdk-language-and-backend-runtime.md) for why it uses TypeScript.

The stack is intentionally empty until [ADR-0002](https://github.com/Bradfordly/bradfordly-games/issues/3) decides the hosting model.

## Prerequisites

- Node.js 24 (`nvm use` reads [.nvmrc](.nvmrc))
- AWS credentials for the target account, only needed for `diff`, `bootstrap`, and `deploy`

## Commands

Run from `infra/`:

| Command | What it does |
|---|---|
| `npm ci` | Install the locked dependencies |
| `npm run build` | Type-check with `tsc` (no output files) |
| `npm test` | Run Jest unit tests |
| `npx cdk synth` | Synthesize CloudFormation into `cdk.out/` |
| `npx cdk diff` | Compare against the deployed stack |
| `npx cdk bootstrap` | One-time setup per account/region before the first deploy |
| `npx cdk deploy` | Deploy the `BradfordlyGames` stack |

## Configuration

The target account and region come from the environment, not from code:

| Variable | Required | Default | Description |
|---|---|---|---|
| `CDK_DEFAULT_ACCOUNT` | No | Set by the CDK CLI from your AWS credentials | Target AWS account ID |
| `CDK_DEFAULT_REGION` | No | Set by the CDK CLI from your AWS profile | Target AWS region |

If neither is set, the stack synthesizes as environment-agnostic.

## Security exceptions

[osv-scanner.toml](osv-scanner.toml) and [.trivyignore.yaml](.trivyignore.yaml) list reviewed vulnerability exceptions. Each entry needs a reason and an expiry date.

`overrides` in [package.json](package.json) forces patched versions of transitive dev dependencies whose parents still pin a vulnerable range. Remove an override once its parents allow the patched version.
