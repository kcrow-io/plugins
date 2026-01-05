# Dependency Management Guide

This document describes the automated dependency management system for the NRI Plugins project.

## Overview

The project uses a multi-layered approach to dependency management:

1. **Dependabot**: GitHub's native dependency update service
2. **Custom GitHub Actions**: Advanced dependency checking and automation
3. **Security Scanning**: Vulnerability detection and reporting
4. **Auto-merge**: Automated PR merging for safe updates

## Configuration

### Dependabot Configuration (`.github/dependabot.yml`)

Dependabot is configured to check for updates in:

- **Go Modules**: Weekly updates for all Go dependencies
- **GitHub Actions**: Weekly updates for workflow actions
- **Docker**: Weekly updates for base images

Key features:
- Groups related dependencies (containerd, kubernetes, golang, grpc)
- Assigns PRs to maintainers
- Uses semantic commit messages
- Limits concurrent PRs to prevent spam

### Advanced Dependency Workflow (`.github/workflows/dependency-update.yml`)

This workflow provides enhanced dependency management:

**Schedule**: Every Monday at 9:00 AM Beijing time (01:00 UTC)

**Features**:
- Detects outdated dependencies using `go-mod-outdated`
- Creates comprehensive update PRs
- Runs full test suite before creating PRs
- Provides detailed update summaries
- Includes security vulnerability scanning

**Manual Trigger**: Can be triggered manually with force update option

### Auto-merge Workflow (`.github/workflows/auto-merge-deps.yml`)

Automatically merges dependency PRs when:
- Created by Dependabot or the dependency update workflow
- All CI checks pass
- No merge conflicts exist

**Safety Features**:
- Waits for all CI checks to complete
- Fails if any tests fail
- Adds detailed merge comments
- Provides fallback notifications if merge fails

## Security

### Vulnerability Scanning

The system includes automated security scanning:

- **govulncheck**: Go-specific vulnerability database
- **SARIF reporting**: Integration with GitHub Security tab
- **Weekly scans**: Regular security audits

### Safe Update Practices

1. **Semantic Versioning**: Respects semver for safe updates
2. **Test Verification**: All updates must pass tests
3. **Build Verification**: Ensures project still builds
4. **Gradual Updates**: Groups related dependencies

## Monitoring and Notifications

### GitHub Actions Summary

Each workflow run provides:
- Update availability status
- Security audit results
- PR creation status
- Auto-merge results

### PR Labels and Organization

Dependency PRs are automatically labeled:
- `dependencies`: All dependency updates
- `go`: Go module updates
- `github-actions`: Workflow updates
- `docker`: Container image updates
- `automated`: Auto-generated PRs

## Manual Operations

### Checking for Updates

```bash
# Install go-mod-outdated tool
go install github.com/psampaz/go-mod-outdated@latest

# Check for outdated dependencies
go list -u -m -json all | go-mod-outdated -update -direct
```

### Manual Updates

```bash
# Update all dependencies to latest compatible versions
go get -u ./...

# Update specific dependency
go get -u github.com/containerd/nri@latest

# Clean up and verify
go mod tidy
go mod verify

# Test the changes
make build
go test ./...
```

### Security Scanning

```bash
# Install govulncheck
go install golang.org/x/vuln/cmd/govulncheck@latest

# Run vulnerability scan
govulncheck ./...
```

## Troubleshooting

### Common Issues

1. **Auto-merge Fails**
   - Check branch protection rules
   - Verify CI checks are passing
   - Look for merge conflicts

2. **Dependency Update Fails**
   - Check for breaking changes in dependencies
   - Review test failures
   - Verify build compatibility

3. **Security Vulnerabilities**
   - Review govulncheck output
   - Check for available patches
   - Consider temporary workarounds

### Manual Intervention

When automatic processes fail:

1. Review the failed workflow logs
2. Check PR comments for specific errors
3. Manually update problematic dependencies
4. Run tests locally before pushing

## Best Practices

### For Maintainers

1. **Review Auto-PRs**: Even automated PRs should be reviewed
2. **Monitor Security**: Pay attention to vulnerability reports
3. **Test Locally**: Test major updates in development environment
4. **Update Documentation**: Keep dependency docs current

### For Contributors

1. **Check Dependencies**: Ensure new dependencies are necessary
2. **Use Semantic Versions**: Pin to appropriate version ranges
3. **Test Compatibility**: Verify changes work with current dependencies
4. **Document Changes**: Note any dependency-related changes

## Configuration Customization

### Adjusting Update Frequency

Edit `.github/dependabot.yml`:

```yaml
schedule:
  interval: "daily"  # or "weekly", "monthly"
  day: "monday"      # for weekly updates
  time: "09:00"
  timezone: "Asia/Shanghai"
```

### Modifying Auto-merge Rules

Edit `.github/workflows/auto-merge-deps.yml` conditions:

```yaml
if: |
  github.actor == 'dependabot[bot]' ||
  (github.actor == 'github-actions[bot]' && contains(github.event.pull_request.title, 'Auto-update'))
```

### Adding Dependency Groups

In `.github/dependabot.yml`:

```yaml
groups:
  my-group:
    patterns:
      - "github.com/myorg/*"
```

## Metrics and Reporting

The system provides metrics through:

- GitHub Actions workflow summaries
- PR comments with update details
- Security scan results
- Dependency update frequency reports

These help track:
- Update success rates
- Security vulnerability trends
- Dependency freshness
- Automation effectiveness