# Mote Security Policy Skill

This skill allows you to manage Mote's security policies and approval requests.

## Available Tools

### mote_policy_status
Get the current security policy configuration.

Returns:
- `default_allow`: Whether tools are allowed by default
- `require_approval`: Whether approval is required for sensitive operations
- `blocklist_count`: Number of blocked tools
- `allowlist_count`: Number of explicitly allowed tools
- `dangerous_rules_count`: Number of dangerous operation rules

### mote_policy_check
Check if a specific tool call would be allowed.

Parameters:
- `tool` (required): The tool name to check (e.g., "shell", "file_write")
- `arguments` (optional): JSON string of the tool arguments to check

Returns:
- `allowed`: Whether the operation is permitted
- `require_approval`: Whether user approval is needed
- `blocked`: Whether the operation is blocked
- `reason`: Explanation of the policy decision

### mote_approval_list
List all pending approval requests.

Returns:
- `pending`: Array of pending approval requests
- `count`: Number of pending requests

Each pending request includes:
- `id`: Request ID
- `tool`: Tool name
- `severity`: Risk level (low, medium, high, critical)
- `message`: Description of the requested operation
- `created_at`: When the request was created

### mote_approval_respond
Respond to a pending approval request.

Parameters:
- `request_id` (required): The ID of the approval request
- `approved` (required): Boolean - true to approve, false to deny
- `reason` (optional): Explanation for the decision

## Default Security Rules

Mote includes these default security rules:

1. **Blocked Operations** (always denied):
   - `rm -rf` commands (critical severity)

2. **Approval Required** (needs user confirmation):
   - `sudo` commands (high severity)

3. **Protected Paths** (write operations blocked):
   - `/etc/*` - System configuration
   - `/usr/*` - System binaries
   - `/bin/*` - Essential binaries

## Usage Examples

Check policy before running a command:
```
Check if running "sudo apt update" would be allowed
```

List pending approvals:
```
Show me any pending security approval requests
```

View current security settings:
```
What are the current security policy settings?
```
