# SQLite Memory Skill

Use this skill to record and retrieve noteworthy information that should persist across different conversation turns, especially when the context is compressed.

## Database Configuration
- **Base Path**: `/tmp/orchard_tmp/memory/`
- **Database File**: `${thread_uuid}.db` (Replace `${thread_uuid}` with the actual ID of the current conversation)
- **Connection**: SQLite (Standard library)

## Schema

### `kanban` Table
| Column | Type | Description |
|---|---|---|
| id | INTEGER | Primary Key |
| task_name | TEXT | Name of the task |
| status | TEXT | Status (e.g., Todo, In Progress, Done) |
| description | TEXT | Details about the task |

### `findings` Table
| Column | Type | Description |
|---|---|---|
| id | INTEGER | Primary Key |
| file_path | TEXT | Path to the file |
| line_range | TEXT | Range of lines (e.g., "10-20") |
| content | TEXT | The information found |

## Usage Instructions
1. Check if the database exists at `/tmp/orchard_tmp/memory/${thread_uuid}.db`.
2. If not, create the database and initialize the two tables.
3. When finding something important, insert it into `findings`.
4. When managing tasks, use the `kanban` table.
5. Before starting a new session or after a long conversation, retrieve the contents of both tables to refresh context.
