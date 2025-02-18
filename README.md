# channelcheck

A static analysis tool that detects potentially dangerous channel operations in Go code.

## What it checks

- Channel sends without select statements (which may block indefinitely)
- Unbuffered channel creation (potential source of deadlocks)

## Usage

```bash
go build

# Check specific file with text output
./channelcheck -path=/path/to/file.go

# Check specific directory with JSON output
./channelcheck -path=/path/to/directory -output=json
```

## Example Output

### Text Output
```
Found 2 potential issues:

[WARNING] /path/to/file.go:15:2: channel send without select statement may block indefinitely
[INFO] /path/to/file.go:10:6: unbuffered channel creation detected - consider specifying buffer size
```

### JSON Output
```json
{
  "total": 2,
  "issues": [
    {
      "severity": "WARNING",
      "message": "channel send without select statement may block indefinitely",
      "file": "/path/to/file.go",
      "line": 15,
      "column": 2,
      "position": "/path/to/file.go:15:2"
    },
    {
      "severity": "INFO",
      "message": "unbuffered channel creation detected - consider specifying buffer size",
      "file": "/path/to/file.go",
      "line": 10,
      "column": 6,
      "position": "/path/to/file.go:10:6"
    }
  ]
}
```
