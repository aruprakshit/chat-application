# WebSocket delivery design

- One selected conversation per WebSocket connection.
- Messages are created through the existing HTTP POST endpoint.
- Events are published only after the database transaction commits.
- PostgreSQL remains the source of truth.
- Clients deduplicate messages by their database ID.
- Switching conversations closes the previous connection.
- Outgoing queues are bounded; slow clients are disconnected.
- Initial implementation supports one backend instance.
- Reconnection reloads recent history; complete replay is out of scope.

Before implementation, define ongoing session and membership validation
and verify that live subscription plus history loading cannot miss messages
during initial connection setup.
