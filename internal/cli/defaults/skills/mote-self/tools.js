// Mote Self-Management Tools
// This skill enables Mote to understand and configure itself.
// Note: mote.http methods are synchronous in goja, do not use async/await

// Use var to allow re-declaration when VM is reused from pool
var BASE_URL = 'http://localhost:18788';

var ALLOWED_KEYS = [
  'copilot.model',
  'copilot.mode',
  'gateway.port',
  'memory.enabled',
  'cron.enabled'
];

/**
 * Get Mote's current configuration settings
 */
function getConfig(args) {
  try {
    var response = mote.http.get(BASE_URL + '/api/v1/config');
    
    if (response.status !== 200) {
      return { error: 'Failed to get config: ' + response.body };
    }
    
    return JSON.parse(response.body);
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Set a Mote configuration value (limited to allowed keys)
 */
function setConfig(args) {
  var key = args.key;
  var value = args.value;
  
  var allowed = false;
  for (var i = 0; i < ALLOWED_KEYS.length; i++) {
    if (ALLOWED_KEYS[i] === key) {
      allowed = true;
      break;
    }
  }
  
  if (!allowed) {
    return {
      error: 'Configuration key not allowed. Allowed keys: ' + ALLOWED_KEYS.join(', ')
    };
  }
  
  try {
    var body = {};
    body[key] = value;
    var response = mote.http.post(BASE_URL + '/api/v1/config', body);
    
    if (response.status !== 200) {
      return { error: 'Failed to set config: ' + response.body };
    }
    
    return { success: true, message: 'Set ' + key + ' = ' + value };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * List all supported models from all enabled providers
 */
function listModels(args) {
  try {
    var response = mote.http.get(BASE_URL + '/api/v1/models');
    
    if (response.status !== 200) {
      return { error: 'Failed to list models: ' + response.body };
    }
    
    return JSON.parse(response.body);
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * List all available AI providers
 */
function listProviders(args) {
  try {
    var response = mote.http.get(BASE_URL + '/api/v1/config');
    
    if (response.status !== 200) {
      return { error: 'Failed to get config: ' + response.body };
    }
    
    var config = JSON.parse(response.body);
    
    // Also get models to include provider status
    var modelsResponse = mote.http.get(BASE_URL + '/api/v1/models');
    var providers = [];
    
    if (modelsResponse.status === 200) {
      var modelsData = JSON.parse(modelsResponse.body);
      if (modelsData.providers) {
        providers = modelsData.providers;
      }
    }
    
    return {
      default: config.provider ? config.provider.default : 'copilot',
      enabled: config.provider ? config.provider.enabled : ['copilot'],
      providers: providers
    };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Switch the current model (supports all providers)
 */
function switchModel(args) {
  var model = args.model;
  
  try {
    // Use the /api/v1/models/current endpoint for model switching
    var response = mote.http.put(BASE_URL + '/api/v1/models/current', {
      model: model
    });
    
    if (response.status !== 200) {
      return { error: 'Failed to switch model: ' + response.body };
    }
    
    return { success: true, message: 'Switched to model: ' + model };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Get or set the current session's model
 */
function sessionModel(args) {
  var sessionId = args.session_id || (mote.context && mote.context.session_id);
  
  if (!sessionId) {
    return { error: 'No session_id provided and no current session context' };
  }
  
  try {
    if (args.model) {
      // Set session model
      var response = mote.http.put(BASE_URL + '/api/v1/sessions/' + sessionId + '/model', {
        model: args.model
      });
      
      if (response.status !== 200) {
        return { error: 'Failed to set session model: ' + response.body };
      }
      
      return { success: true, message: 'Session model set to: ' + args.model };
    } else {
      // Get session model
      var response = mote.http.get(BASE_URL + '/api/v1/sessions/' + sessionId);
      
      if (response.status !== 200) {
        return { error: 'Failed to get session: ' + response.body };
      }
      
      var session = JSON.parse(response.body);
      return {
        session_id: sessionId,
        model: session.model || '(inherited from scenario)',
        scenario: session.scenario || 'chat'
      };
    }
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Get scenario default models
 */
function getScenarioModels(args) {
  try {
    var response = mote.http.get(BASE_URL + '/api/v1/settings/models');
    
    if (response.status !== 200) {
      return { error: 'Failed to get scenario models: ' + response.body };
    }
    
    return JSON.parse(response.body);
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Set a scenario's default model
 */
function setScenarioModel(args) {
  var scenario = args.scenario;
  var model = args.model;
  
  var validScenarios = ['chat', 'cron', 'channel'];
  var isValid = false;
  for (var i = 0; i < validScenarios.length; i++) {
    if (validScenarios[i] === scenario) {
      isValid = true;
      break;
    }
  }
  
  if (!isValid) {
    return { error: 'Invalid scenario. Valid values: ' + validScenarios.join(', ') };
  }
  
  try {
    var body = {};
    body[scenario] = model;
    var response = mote.http.put(BASE_URL + '/api/v1/settings/models', body);
    
    if (response.status !== 200) {
      return { error: 'Failed to set scenario model: ' + response.body };
    }
    
    return { success: true, message: 'Set ' + scenario + ' default model to: ' + model };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Get Mote version and health information
 */
function getVersion(args) {
  try {
    var response = mote.http.get(BASE_URL + '/api/v1/health');
    
    if (response.status !== 200) {
      return { error: 'Failed to get version: ' + response.body };
    }
    
    var data = JSON.parse(response.body);
    return { 
      version: data.version,
      status: data.status
    };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * List all available tools in Mote
 */
function listTools(args) {
  try {
    var response = mote.http.get(BASE_URL + '/api/v1/tools');
    
    if (response.status !== 200) {
      return { error: 'Failed to list tools: ' + response.body };
    }
    
    var data = JSON.parse(response.body);
    var tools = data.tools || [];
    
    // Group tools by type/source
    var builtinTools = [];
    var skillTools = [];
    var mcpTools = [];
    
    for (var i = 0; i < tools.length; i++) {
      var tool = tools[i];
      var name = tool.name;
      
      if (name.indexOf('mcp_') === 0) {
        mcpTools.push(name);
      } else if (name.indexOf('mote_') === 0) {
        skillTools.push(name);
      } else {
        builtinTools.push(name);
      }
    }
    
    return {
      total: tools.length,
      builtin: builtinTools,
      skills: skillTools,
      mcp: mcpTools
    };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * List all registered skills in Mote
 */
function listSkills(args) {
  // Note: There's no skills API endpoint yet, so we return known skills
  return {
    skills: [
      { id: 'mote-self', name: 'Mote Self Management', tools: 7 },
      { id: 'mote-memory', name: 'Mote Memory Management', tools: 4 },
      { id: 'mote-cron', name: 'Mote Cron Scheduler', tools: 6 },
      { id: 'mote-mcp-config', name: 'Mote MCP Configuration', tools: 0 },
      { id: 'mote-security', name: 'Mote Security Policy', tools: 4 }
    ],
    note: 'Skills API endpoint coming soon'
  };
}

/**
 * Bind a workspace directory to the current session
 */
function workspaceBind(args) {
  var path = args.path;
  var sessionId = args.session_id || (mote.context && mote.context.session_id);
  var readOnly = args.read_only || false;
  
  if (!path) {
    return { error: 'path is required' };
  }
  
  if (!sessionId) {
    return { error: 'No session_id provided and no current session context' };
  }
  
  try {
    var response = mote.http.post(BASE_URL + '/api/v1/workspaces', {
      session_id: sessionId,
      path: path,
      read_only: readOnly
    });
    
    if (response.status !== 200 && response.status !== 201) {
      return { error: 'Failed to bind workspace: ' + response.body };
    }
    
    return JSON.parse(response.body);
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Unbind the workspace from the current session
 */
function workspaceUnbind(args) {
  var sessionId = args.session_id || (mote.context && mote.context.session_id);
  
  if (!sessionId) {
    return { error: 'No session_id provided and no current session context' };
  }
  
  try {
    var response = mote.http.delete(BASE_URL + '/api/v1/workspaces/' + sessionId);
    
    if (response.status !== 200 && response.status !== 204) {
      return { error: 'Failed to unbind workspace: ' + response.body };
    }
    
    return { success: true, message: 'Workspace unbound from session ' + sessionId };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * List all bound workspaces
 */
function workspaceList(args) {
  try {
    var response = mote.http.get(BASE_URL + '/api/v1/workspaces');
    
    if (response.status !== 200) {
      return { error: 'Failed to list workspaces: ' + response.body };
    }
    
    var data = JSON.parse(response.body);
    // Format output for clarity
    var workspaces = data.workspaces || [];
    var result = {
      total: workspaces.length,
      workspaces: workspaces.map(function(ws) {
        return {
          session_id: ws.session_id,
          path: ws.path,
          alias: ws.alias || '',
          directory_name: ws.path.split('/').pop() || ws.path,
          read_only: ws.read_only,
          bound_at: ws.bound_at
        };
      })
    };
    return result;
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * List files in the current workspace
 */
function workspaceFiles(args) {
  var sessionId = args.session_id || (mote.context && mote.context.session_id);
  var path = args.path || '';
  
  if (!sessionId) {
    return { error: 'No session_id provided and no current session context' };
  }
  
  try {
    var url = BASE_URL + '/api/v1/workspaces/' + sessionId + '/files';
    if (path) {
      url = url + '?path=' + encodeURIComponent(path);
    }
    
    var response = mote.http.get(url);
    
    if (response.status !== 200) {
      return { error: 'Failed to list workspace files: ' + response.body };
    }
    
    var data = JSON.parse(response.body);
    return data;
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Get current session's workspace binding
 */
function workspaceGet(args) {
  var sessionId = args.session_id || (mote.context && (mote.context && mote.context.session_id));
  
  if (!sessionId) {
    return { error: 'No session_id provided and no current session context' };
  }
  
  try {
    var response = mote.http.get(BASE_URL + '/api/v1/workspaces/' + sessionId);
    
    if (response.status === 404) {
      return { bound: false, message: 'No workspace bound to current session' };
    }
    
    if (response.status !== 200) {
      return { error: 'Failed to get workspace: ' + response.body };
    }
    
    var ws = JSON.parse(response.body);
    return {
      bound: true,
      session_id: ws.session_id,
      path: ws.path,
      directory_name: ws.path.split('/').pop() || ws.path,
      alias: ws.alias || '',
      read_only: ws.read_only,
      bound_at: ws.bound_at
    };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

// Note: No module.exports needed - functions are called directly by name
