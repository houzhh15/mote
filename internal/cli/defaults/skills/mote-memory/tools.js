// Mote Memory Management Tools
// This skill enables Mote to search, add, and manage memories.
// Note: mote.http methods are synchronous in goja, do not use async/await

// Use var to allow re-declaration when VM is reused from pool
var BASE_URL = 'http://localhost:18788';

/**
 * Search for memories using semantic similarity
 */
function memorySearch(args) {
  var query = args.query;
  var limit = args.limit || 10;
  var threshold = args.threshold || 0;
  // P2: New filter parameters
  var categories = args.categories;
  var minImportance = args.min_importance;
  
  if (!query) {
    return { error: 'Query is required' };
  }

  try {
    var body = {
      query: query,
      top_k: limit
    };
    
    // P2: Add filter parameters if provided
    if (categories && categories.length > 0) {
      body.categories = categories;
    }
    if (minImportance && minImportance > 0) {
      body.min_importance = minImportance;
    }
    
    var response = mote.http.post(BASE_URL + '/api/v1/memory/search', body);
    
    if (response.status !== 200) {
      return { error: 'Search failed: ' + response.body };
    }
    
    var data = JSON.parse(response.body);
    return {
      results: data.results || [],
      count: (data.results || []).length
    };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Add a new memory entry
 */
function memoryAdd(args) {
  var content = args.content;
  var source = args.source || 'manual';
  // P2: New parameters
  var category = args.category;
  var importance = args.importance;
  
  if (!content) {
    return { error: 'Content is required' };
  }

  try {
    var body = {
      content: content,
      source: source,
      capture_method: 'auto'
    };
    
    // P2: Add optional parameters if provided
    if (category) {
      body.category = category;
    }
    if (importance && importance > 0) {
      body.importance = importance;
    }
    
    var response = mote.http.post(BASE_URL + '/api/v1/memory', body);
    
    if (response.status !== 200 && response.status !== 201) {
      return { error: 'Failed to add memory: ' + response.body };
    }
    
    var data = JSON.parse(response.body);
    return { 
      success: true, 
      message: 'Memory added successfully',
      id: data.id,
      category: data.category,
      source: source
    };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * List recent memories
 */
function memoryList(args) {
  args = args || {};
  var limit = args.limit || 20;

  try {
    // Use a broad search to list recent memories
    var response = mote.http.post(BASE_URL + '/api/v1/memory/search', {
      query: '*',
      limit: limit,
      threshold: 0
    });
    
    if (response.status !== 200) {
      return { error: 'List failed: ' + response.body };
    }
    
    var data = JSON.parse(response.body);
    return {
      memories: data.results || [],
      count: (data.results || []).length
    };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Get a specific memory entry by ID
 */
function memoryGet(args) {
  var id = args.id;
  
  if (!id) {
    return { error: 'Memory ID is required' };
  }

  try {
    var response = mote.http.get(BASE_URL + '/api/v1/memory/' + encodeURIComponent(id));
    
    if (response.status === 404) {
      return { error: 'Memory not found', id: id };
    }
    
    if (response.status !== 200) {
      return { error: 'Failed to get memory: ' + response.body };
    }
    
    var data = JSON.parse(response.body);
    return {
      id: data.id,
      content: data.content,
      source: data.source,
      created_at: data.created_at,
      metadata: data.metadata || {},
      // P2 fields
      category: data.category,
      importance: data.importance,
      capture_method: data.capture_method
    };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Delete a specific memory by ID
 */
function memoryDelete(args) {
  var id = args.id;
  
  if (!id) {
    return { error: 'Memory ID is required' };
  }

  try {
    var response = mote.http.delete(BASE_URL + '/api/v1/memory/' + id);
    
    if (response.status !== 200 && response.status !== 204) {
      return { error: 'Failed to delete memory: ' + response.body };
    }
    
    return { 
      success: true, 
      message: 'Memory deleted successfully'
    };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Synchronize memories with external markdown storage (P1)
 */
function memorySync(args) {
  try {
    var response = mote.http.post(BASE_URL + '/api/v1/memory/sync', {});
    
    if (response.status !== 200) {
      return { error: 'Sync failed: ' + response.body };
    }
    
    var data = JSON.parse(response.body);
    return {
      success: true,
      message: 'Memory sync completed',
      synced: data.synced || 0
    };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Get or append to daily memory log (P1)
 */
function memoryDaily(args) {
  args = args || {};
  var content = args.content;
  
  try {
    if (content) {
      // POST mode: append to daily log
      var response = mote.http.post(BASE_URL + '/api/v1/memory/daily', {
        content: content
      });
      
      if (response.status !== 200 && response.status !== 201) {
        return { error: 'Failed to append to daily log: ' + response.body };
      }
      
      return {
        success: true,
        message: 'Added to daily log'
      };
    } else {
      // GET mode: retrieve daily log
      var response = mote.http.get(BASE_URL + '/api/v1/memory/daily');
      
      if (response.status !== 200) {
        return { error: 'Failed to get daily log: ' + response.body };
      }
      
      var data = JSON.parse(response.body);
      return {
        date: data.date,
        entries: data.entries || [],
        count: (data.entries || []).length
      };
    }
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Export all memories (P1)
 */
function memoryExport(args) {
  args = args || {};
  var format = args.format || 'json';
  
  try {
    var response = mote.http.get(BASE_URL + '/api/v1/memory/export?format=' + encodeURIComponent(format));
    
    if (response.status !== 200) {
      return { error: 'Export failed: ' + response.body };
    }
    
    var data = JSON.parse(response.body);
    return {
      success: true,
      format: format,
      count: data.count || 0,
      data: data.memories || data
    };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Import memories from JSON (P1)
 */
function memoryImport(args) {
  var data = args.data;
  
  if (!data) {
    return { error: 'Import data is required' };
  }
  
  try {
    var memories;
    if (typeof data === 'string') {
      memories = JSON.parse(data);
    } else {
      memories = data;
    }
    
    var response = mote.http.post(BASE_URL + '/api/v1/memory/import', {
      memories: memories
    });
    
    if (response.status !== 200) {
      return { error: 'Import failed: ' + response.body };
    }
    
    var result = JSON.parse(response.body);
    return {
      success: true,
      message: 'Memories imported successfully',
      imported: result.imported || 0
    };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

/**
 * Get memory statistics (P2)
 */
function memoryStats(args) {
  try {
    var response = mote.http.get(BASE_URL + '/api/v1/memory/stats');
    
    if (response.status !== 200) {
      return { error: 'Failed to get stats: ' + response.body };
    }
    
    var data = JSON.parse(response.body);
    return {
      total: data.total || 0,
      by_category: data.by_category || {},
      by_capture_method: data.by_capture_method || {},
      auto_capture_today: data.auto_capture_today || 0,
      auto_recall_today: data.auto_recall_today || 0
    };
  } catch (err) {
    return { error: err.message || String(err) };
  }
}

// Note: No module.exports needed - functions are called directly by name
