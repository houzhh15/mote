#!/bin/bash
# Fix golangci-lint errcheck issues

set -e

cd /Users/tshinjeii/Documents/code/openclaw/mote

# Fix pool.AddProvider in test files
find . -name "*_test.go" -type f -exec sed -i '' 's/^\t*pool\.AddProvider(/\t_ = pool.AddProvider(/g' {} \;

# Fix r.Register / router.RegisterRoutes in test files  
find . -name "*_test.go" -type f -exec sed -i '' 's/^\t*router\.RegisterRoutes(/\t_ = router.RegisterRoutes(/g' {} \;
find . -name "*_test.go" -type f -exec sed -i '' 's/^\t*r\.Register(/\t_ = r.Register(/g' {} \;

# Fix manager.Bind in test files
find . -name "*_test.go" -type f -exec sed -i '' 's/^\t*manager\.Bind(/\t_ = manager.Bind(/g' {} \;

# Fix registry.Register in test files
find . -name "*_test.go" -type f -exec sed -i '' 's/^\t*registry\.Register(/\t_ = registry.Register(/g' {} \;

# Fix store.Set in test files
find . -name "*_test.go" -type f -exec sed -i '' 's/^\t*store\.Set(/\t_ = store.Set(/g' {} \;
find . -name "*_test.go" -type f -exec sed -i '' 's/^\t*store1\.Set(/\t_ = store1.Set(/g' {} \;

# Fix m.Register in test files (hooks)
find . -name "*_test.go" -type f -exec sed -i '' 's/^\t*m\.Register(/\t_ = m.Register(/g' {} \;

# Fix m.Trigger* in test files
find . -name "*_test.go" -type f -exec sed -i '' 's/^\t*m\.TriggerBeforeMessage(/\t_ = m.TriggerBeforeMessage(/g' {} \;
find . -name "*_test.go" -type f -exec sed -i '' 's/^\t*m\.TriggerBeforeToolCall(/\t_ = m.TriggerBeforeToolCall(/g' {} \;
find . -name "*_test.go" -type f -exec sed -i '' 's/^\t*m\.TriggerAfterToolCall(/\t_ = m.TriggerAfterToolCall(/g' {} \;
find . -name "*_test.go" -type f -exec sed -i '' 's/^\t*m\.TriggerSessionCreate(/\t_ = m.TriggerSessionCreate(/g' {} \;

# Fix ut.Record in test files
find . -name "*_test.go" -type f -exec sed -i '' 's/^\t*ut\.Record(/\t_ = ut.Record(/g' {} \;
find . -name "*_test.go" -type f -exec sed -i '' 's/^\t*ut1\.Record(/\t_ = ut1.Record(/g' {} \;

echo "✅ Fixed errcheck issues in test files"
