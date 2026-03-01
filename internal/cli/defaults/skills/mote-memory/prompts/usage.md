# Mote Memory System

Mote has a built-in memory system that allows storing and retrieving information using semantic search, with automatic capture and recall capabilities.

## Memory Operations

### Searching Memories
Use `mote_memory_search` to find relevant memories:
- **query**: What to search for
- **limit**: Maximum results (default: 10)
- **threshold**: Minimum similarity score 0-1 (default: 0)
- **categories**: Filter by category array: "preference", "fact", "decision", "entity", "other"
- **min_importance**: Minimum importance threshold 0-1

### Adding Memories
Use `mote_memory_add` to store new information:
- **content**: The information to remember
- **source**: Where it came from (manual, conversation, document, tool)
- **category**: Memory category (auto-detected if not provided): preference, fact, decision, entity, other
- **importance**: Importance score 0-1 (default: 0.7)

### Getting Memory Details
Use `mote_memory_get` with an ID to retrieve full details including category, importance, and capture method.

### Listing Memories
Use `mote_memory_list` to see recent memories.

### Deleting Memories
Use `mote_memory_delete` with a memory ID to remove it.

### Memory Statistics
Use `mote_memory_stats` to view statistics:
- Total memory count
- Count by category (preference, fact, decision, entity, other)
- Count by capture method (manual, auto, import)
- Today's auto-capture count
- Today's auto-recall count

## Memory Categories

- **preference**: User preferences (likes, dislikes, settings)
- **fact**: Facts about the user (name, location, occupation)
- **decision**: Decisions or choices made
- **entity**: Important entities (projects, people, places)
- **other**: General information

## Auto-Capture & Auto-Recall

The memory system can automatically:
1. **Auto-Capture**: Detect and save important information from conversations
2. **Auto-Recall**: Inject relevant memories into context when processing user input

These features can be configured in the mote config file.

## Best Practices

1. **Store important facts**: When the user shares important information, save it as a memory
2. **Search before asking**: Check memories before asking the user for information they may have already provided
3. **Use meaningful content**: Store clear, concise descriptions that will be easy to find later
4. **Use categories**: Specify categories for better organization and filtering
5. **Set importance**: Higher importance memories are prioritized in recall

## Example Usage

```javascript
// Search for memories about the user's project
mote_memory_search({ query: "user project preferences" })

// Search only preferences with high importance
mote_memory_search({ 
  query: "user preferences", 
  categories: ["preference"], 
  min_importance: 0.8 
})

// Store a user preference with category
mote_memory_add({ 
  content: "User prefers TypeScript over JavaScript", 
  source: "conversation",
  category: "preference",
  importance: 0.9
})

// List recent memories
mote_memory_list({ limit: 5 })

// View memory statistics
mote_memory_stats({})
```
