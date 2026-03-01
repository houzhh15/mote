// Mote Cron Scheduler Tools
// This skill enables Mote to create and manage scheduled tasks.
// Note: mote.http methods are synchronous in goja, do not use async/await

// Use var to allow re-declaration when VM is reused from pool
var BASE_URL = 'http://localhost:18788';

/**
 * List all scheduled cron jobs
 */
function cronList(args) {
  try {
    var response = mote.http.get(BASE_URL + '/api/v1/cron/jobs');
    
    if (response.status !== 200) {
      return { error: 'Failed to list cron jobs: ' + response.body };
    }
    
    var data = JSON.parse(response.body);
    return {
      jobs: data.jobs || [],
      count: (data.jobs || []).length
    };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Create a new scheduled cron job
 */
function cronAdd(args) {
  var name = args.name;
  var schedule = args.schedule;
  var type = args.type || 'prompt';
  var message = args.message;
  var tool = args.tool;
  var enabled = args.enabled !== false;
  
  if (!name) {
    return { error: 'Job name is required' };
  }
  if (!schedule) {
    return { error: 'Schedule is required' };
  }

  // Build payload based on job type
  var payload = {};
  if (type === 'prompt') {
    if (!message) {
      return { error: 'Message is required for prompt type' };
    }
    payload = { message: message };
  } else if (type === 'tool') {
    if (!tool) {
      return { error: 'Tool name is required for tool type' };
    }
    payload = { tool: tool };
  } else if (type === 'script') {
    payload = {};
  } else {
    return { error: 'Invalid job type: ' + type + '. Must be prompt, tool, or script' };
  }

  try {
    var response = mote.http.post(BASE_URL + '/api/v1/cron/jobs', {
      name: name,
      schedule: schedule,
      type: type,
      payload: JSON.stringify(payload),
      enabled: enabled
    });
    
    if (response.status !== 200 && response.status !== 201) {
      return { error: 'Failed to create cron job: ' + response.body };
    }
    
    return {
      success: true,
      message: 'Cron job created successfully',
      schedule: schedule,
      type: type
    };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Delete a scheduled cron job
 */
function cronRemove(args) {
  var name = args.name;
  
  if (!name) {
    return { error: 'Job name is required' };
  }

  try {
    var response = mote.http.delete(BASE_URL + '/api/v1/cron/jobs/' + encodeURIComponent(name));
    
    if (response.status !== 200 && response.status !== 204) {
      return { error: 'Failed to delete cron job: ' + response.body };
    }
    
    return {
      success: true,
      message: 'Cron job deleted successfully'
    };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Trigger a cron job to run immediately
 */
function cronRun(args) {
  var name = args.name;
  
  if (!name) {
    return { error: 'Job name is required' };
  }

  try {
    var response = mote.http.post(BASE_URL + '/api/v1/cron/jobs/' + encodeURIComponent(name) + '/run', {});
    
    if (response.status !== 200) {
      return { error: 'Failed to run cron job: ' + response.body };
    }
    
    var data = JSON.parse(response.body);
    return {
      success: true,
      message: 'Cron job triggered successfully',
      result: data
    };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Get details of a specific cron job
 */
function cronGet(args) {
  var name = args.name;
  
  if (!name) {
    return { error: 'Job name is required' };
  }

  try {
    var response = mote.http.get(BASE_URL + '/api/v1/cron/jobs/' + encodeURIComponent(name));
    
    if (response.status !== 200) {
      return { error: 'Failed to get cron job: ' + response.body };
    }
    
    return JSON.parse(response.body);
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Get execution history for cron jobs
 */
function cronHistory(args) {
  args = args || {};
  var job_name = args.job_name;
  var limit = args.limit || 20;

  try {
    var url = BASE_URL + '/api/v1/cron/history?limit=' + limit;
    if (job_name) {
      url += '&job_name=' + encodeURIComponent(job_name);
    }
    
    var response = mote.http.get(url);
    
    if (response.status !== 200) {
      return { error: 'Failed to get history: ' + response.body };
    }
    
    var data = JSON.parse(response.body);
    return {
      history: data.history || [],
      count: (data.history || []).length
    };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

// Note: No module.exports needed - functions are called directly by name
