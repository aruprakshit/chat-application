"use client";

import { useEffect, useRef, useState } from "react";
import MessageComposer from "./message-composer";
import type { Message } from "@/types/message";

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
  | {
      status: "success";
      messages: Message[];
      nextCursor: number | null;
    }
  | { status: "unavailable" }
  | { status: "error" };

export default function MessageHistory({
  conversationId,
  currentUserId,
  onSessionExpired,
}: MessageHistoryProps) {
  const [state, setState] = useState<LoadState>({ status: "loading" });
  const [attempt, setAttempt] = useState(0);
  const [isLoadingOlder, setIsLoadingOlder] = useState(false);
  const [olderError, setOlderError] = useState("");

  // Initially: no request running
  const olderRequest = useRef<AbortController | null>(null);

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
            nextCursor: page.next_cursor,
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
    return () => {
      controller.abort();
      olderRequest.current?.abort();
    };
  }, [conversationId, attempt, onSessionExpired]);

  function retry() {
    setState({ status: "loading" });
    setAttempt((previous) => previous + 1);
  }

  async function loadOlder() {
    // 1. REQUIRE A CURSOR AND PREVENT OVERLAPPING REQUESTS
    if (
      state.status !== "success" ||
      state.nextCursor === null ||
      olderRequest.current !== null
    ) {
      return;
    }

    const cursor = state.nextCursor;
    const controller = new AbortController();

    // Before fetch: mark a request as running
    olderRequest.current = controller;
    setIsLoadingOlder(true);
    setOlderError("");

    try {
      // 2. REQUEST THE PAGE BEFORE OUR CURRENT OLDEST MESSAGE
      const response = await fetch(
        `/api/conversations/${conversationId}/messages?limit=20&before_id=${cursor}`,
        {
          credentials: "same-origin",
          cache: "no-store",
          signal: controller.signal,
        },
      );

      if (controller.signal.aborted) {
        return;
      }

      if (response.status === 401) {
        onSessionExpired();
        return;
      }

      if (response.status === 404) {
        setState({ status: "unavailable" });
        return;
      }

      if (!response.ok) {
        throw new Error("Older messages request failed");
      }

      const page: MessagesResponse = await response.json();

      if (controller.signal.aborted) {
        return;
      }

      // 3. PREPEND OLDER MESSAGES WITHOUT DUPLICATING EXISTING IDS
      setState((current) => {
        if (current.status !== "success") {
          return current;
        }

        const seen = new Set(current.messages.map((message) => message.id));

        const olderMessages = [...page.messages].reverse().filter((message) => {
          if (seen.has(message.id)) {
            return false;
          }

          seen.add(message.id);
          return true;
        });

        return {
          status: "success",
          messages: [...olderMessages, ...current.messages],
          nextCursor: page.next_cursor,
        };
      });
    } catch {
      if (!controller.signal.aborted) {
        setOlderError("Could not load older messages. Please try again.");
      }
    } finally {
      // 4. RELEASE THE REQUEST GUARD
      if (olderRequest.current === controller) {
        // In finally: clear it when that request finishes
        olderRequest.current = null;
      }

      if (!controller.signal.aborted) {
        setIsLoadingOlder(false);
      }
    }
  }

  function handleMessageSent(message: Message) {
    // Ignore an unexpected response for another conversation.
    if (message.conversation_id !== conversationId) {
      return;
    }

    setState((current) => {
      if (current.status !== "success") {
        return current;
      }

      // Avoid inserting the same server-generated ID twice.
      if (current.messages.some((existing) => existing.id === message.id)) {
        return current;
      }

      return {
        ...current,
        messages: [...current.messages, message].sort(
          (left, right) => left.id - right.id,
        ),
      };
    });
  }

  function handleConversationUnavailable() {
    setState({ status: "unavailable" });
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

  // 6. DISPLAY MESSAGE CONTENT AND AUTHOR INFORMATION
  return (
    <div>
      {state.nextCursor !== null && (
        <button type="button" onClick={loadOlder} disabled={isLoadingOlder}>
          {isLoadingOlder ? "Loading older messages…" : "Load older messages"}
        </button>
      )}

      {olderError && <p role="alert">{olderError}</p>}
      {state.messages.length === 0 && (
        <p>No messages in this conversation yet.</p>
      )}
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
      <MessageComposer
        conversationId={conversationId}
        onMessageSent={handleMessageSent}
        onSessionExpired={onSessionExpired}
        onConversationUnavailable={handleConversationUnavailable}
      />
    </div>
  );
}
