# Mote Cron Scheduler

Mote has a built-in cron scheduler for automating tasks on a schedule.

## Cron Expression Format

Standard 5-field cron format:
```
┌───────────── minute (0 - 59)
│ ┌───────────── hour (0 - 23)
│ │ ┌───────────── day of month (1 - 31)
│ │ │ ┌───────────── month (1 - 12)
│ │ │ │ ┌───────────── day of week (0 - 6, Sunday = 0)
│ │ │ │ │
* * * * *
```

Also supports 6-field format with seconds:
```
┌───────────── second (0 - 59)
│ ┌───────────── minute (0 - 59)
│ │ ┌───────────── hour (0 - 23)
│ │ │ ┌───────────── day of month (1 - 31)
│ │ │ │ ┌───────────── month (1 - 12)
│ │ │ │ │ ┌───────────── day of week (0 - 6)
│ │ │ │ │ │
* * * * * *
```

## Common Patterns

| Expression | Description |
|------------|-------------|
| `0 9 * * *` | Every day at 9:00 AM |
| `*/5 * * * *` | Every 5 minutes |
| `0 0 * * 0` | Every Sunday at midnight |
| `0 8-18 * * 1-5` | Every hour 8 AM - 6 PM on weekdays |
| `0 0 1 * *` | First day of every month at midnight |

## Job Types

### Prompt Jobs
Send a message to the agent at the scheduled time:
```javascript
mote_cron_add({
  name: "daily_standup",
  schedule: "0 9 * * 1-5",
  type: "prompt",
  message: "Good morning! What tasks should we focus on today?"
})
```

### Tool Jobs
Execute a registered tool at the scheduled time:
```javascript
mote_cron_add({
  name: "health_check",
  schedule: "*/30 * * * *",
  type: "tool",
  tool: "system_health_check"
})
```

### Script Jobs
Run a JavaScript file at the scheduled time:
```javascript
mote_cron_add({
  name: "backup_job",
  schedule: "0 2 * * *",
  type: "script"
})
```

## Operations

### List Jobs
```javascript
mote_cron_list()
```

### Create Job
```javascript
mote_cron_add({
  name: "reminder",
  schedule: "0 14 * * *",
  type: "prompt",
  message: "Time for your afternoon break!"
})
```

### Delete Job
```javascript
mote_cron_remove({ name: "old_job" })
```

### Run Immediately
```javascript
mote_cron_run({ name: "test_job" })
```

### View History
```javascript
mote_cron_history({ limit: 10 })
```
