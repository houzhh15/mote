// Mote Security Policy Tools
// This skill enables Mote to manage security policies and approval requests.
// Note: mote.http methods are synchronous in goja, do not use async/await

// Use var to allow re-declaration when VM is reused from pool
var BASE_URL = 'http://localhost:18788';

/**
 * Get current security policy status
 */
function policyStatus(args) {
  try {
    var response = mote.http.get(BASE_URL + '/api/v1/policy/status');
    
    if (response.status !== 200) {
      // If endpoint doesn't exist yet, return default info
      if (response.status === 404) {
        return {
          default_allow: true,
          require_approval: false,
          blocklist_count: 0,
          allowlist_count: 0,
          dangerous_rules_count: 3,
          note: 'Policy API not yet implemented, showing defaults'
        };
      }
      return { error: 'Failed to get policy status: ' + response.body };
    }
    
    return JSON.parse(response.body);
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Check if a tool call would be allowed
 */
function policyCheck(args) {
  var tool = args.tool;
  var arguments_str = args.arguments || '{}';
  
  if (!tool) {
    return { error: 'Tool name is required' };
  }

  try {
    var response = mote.http.post(BASE_URL + '/api/v1/policy/check', {
      tool: tool,
      arguments: arguments_str
    });
    
    if (response.status !== 200) {
      // If endpoint doesn't exist, simulate basic check
      if (response.status === 404) {
        // Check against known dangerous patterns
        var dangerous = false;
        var reason = '';
        
        if (tool === 'shell') {
          if (arguments_str.indexOf('rm -rf') !== -1) {
            dangerous = true;
            reason = 'rm -rf is prohibited';
          } else if (arguments_str.indexOf('sudo') !== -1) {
            dangerous = true;
            reason = 'sudo requires approval';
          }
        }
        
        return {
          tool: tool,
          allowed: !dangerous,
          require_approval: dangerous && arguments_str.indexOf('sudo') !== -1,
          blocked: dangerous && arguments_str.indexOf('rm -rf') !== -1,
          reason: reason || 'Allowed by default policy',
          note: 'Policy API not yet implemented, using local check'
        };
      }
      return { error: 'Failed to check policy: ' + response.body };
    }
    
    return JSON.parse(response.body);
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * List pending approval requests
 */
function approvalList(args) {
  try {
    var response = mote.http.get(BASE_URL + '/api/v1/approvals');
    
    if (response.status !== 200) {
      if (response.status === 404) {
        return {
          pending: [],
          count: 0,
          note: 'Approval API not yet implemented'
        };
      }
      return { error: 'Failed to list approvals: ' + response.body };
    }
    
    var data = JSON.parse(response.body);
    return {
      pending: data.pending || [],
      count: (data.pending || []).length
    };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Respond to an approval request
 */
function approvalRespond(args) {
  var request_id = args.request_id;
  var approved = args.approved;
  var reason = args.reason || '';
  
  if (!request_id) {
    return { error: 'Request ID is required' };
  }
  if (approved === undefined) {
    return { error: 'Approved (true/false) is required' };
  }

  try {
    var response = mote.http.post(BASE_URL + '/api/v1/approvals/' + request_id + '/respond', {
      approved: approved,
      reason: reason
    });
    
    if (response.status !== 200) {
      if (response.status === 404) {
        return {
          error: 'Approval request not found or API not implemented',
          request_id: request_id
        };
      }
      return { error: 'Failed to respond: ' + response.body };
    }
    
    return {
      success: true,
      request_id: request_id,
      approved: approved,
      message: approved ? 'Request approved' : 'Request denied'
    };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

// Note: No module.exports needed - functions are called directly by name
