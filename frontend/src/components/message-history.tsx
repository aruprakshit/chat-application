"use client";

import { useEffect, useState } from "react";

type Message = {
  id: number;
  conversation_id: number;
  sender_id: number;
  body: string;
  created_at: string;
};

type MessagesResponse = {
  messages: Message[];
  next_cursor: number | null;
};

type MessageHistoryProps = {
  conversationId: number;
  currentUserId: number;
  onSessionExpired: () => void;
};

type LoadState =
  | { status: "loading" }
  | { status: "success"; messages: Message[] }
  | { status: "unavailable" }
  | { status: "error" };

export default function MessageHistory({
  conversationId,
  currentUserId,
  onSessionExpired,
}: MessageHistoryProps) {
  const [state, setState] = useState<LoadState>({ status: "loading" });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    // 1. CREATE A REQUEST THAT CAN BE CANCELED
    const controller = new AbortController();

    async function loadMessages() {
      try {
        const response = await fetch(
          `/api/conversations/${conversationId}/messages?limit=20`,
          {
            credentials: "same-origin",
            cache: "no-store",
            signal: controller.signal,
          },
        );

        if (controller.signal.aborted) {
          return;
        }

        // 2. HANDLE AUTHENTICATION AND ACCESS FAILURES
        if (response.status === 401) {
          onSessionExpired();
          return;
        }

        if (response.status === 404) {
          setState({ status: "unavailable" });
          return;
        }

        if (!response.ok) {
          throw new Error("Message history request failed");
        }

        const page: MessagesResponse = await response.json();

        // 3. DISPLAY OLDEST FIRST WITHOUT MUTATING THE API ARRAY
        if (!controller.signal.aborted) {
          setState({
            status: "success",
            messages: [...page.messages].reverse(),
          });
        }
      } catch {
        if (!controller.signal.aborted) {
          setState({ status: "error" });
        }
      }
    }

    void loadMessages();

    // 4. CANCEL WHEN THE COMPONENT IS REMOVED OR THE EFFECT RESTARTS
    return () => controller.abort();
  }, [conversationId, attempt, onSessionExpired]);

  function retry() {
    setState({ status: "loading" });
    setAttempt((previous) => previous + 1);
  }

  // 5. RENDER REQUEST STATES
  if (state.status === "loading") {
    return <p role="status">Loading messages…</p>;
  }

  if (state.status === "unavailable") {
    return (
      <p role="alert">
        This conversation is unavailable. Select another conversation.
      </p>
    );
  }

  if (state.status === "error") {
    return (
      <div>
        <p role="alert">Could not load messages. Please try again.</p>
        <button type="button" onClick={retry}>
          Try again
        </button>
      </div>
    );
  }

  if (state.messages.length === 0) {
    return <p>No messages in this conversation yet.</p>;
  }

  // 6. DISPLAY MESSAGE CONTENT AND AUTHOR INFORMATION
  return (
    <ol style={{ listStyle: "none", padding: 0 }}>
      {state.messages.map((message) => {
        const isMine = message.sender_id === currentUserId;

        return (
          <li
            key={message.id}
            style={{
              marginBottom: "1rem",
              padding: "0.75rem",
              border: "1px solid currentColor",
              borderInlineStartWidth: isMine ? "4px" : "1px",
            }}
          >
            <p>
              <strong>{isMine ? "You" : `User #${message.sender_id}`}</strong>
            </p>

            <p style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>
              {message.body}
            </p>

            <time dateTime={message.created_at}>
              {new Date(message.created_at).toLocaleString()}
            </time>
          </li>
        );
      })}
    </ol>
  );
}
