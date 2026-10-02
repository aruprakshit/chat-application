"use client";

import { useEffect, useState } from "react";
import ConversationList, { type Conversation } from "./conversation-list";

type ConversationsPanelProps = {
  onSessionExpired: () => void;
};

// Each state carries only the information needed for that state.
type LoadState =
  | { status: "loading" }
  | { status: "success"; conversations: Conversation[] }
  | { status: "error"; message: string };

export default function ConversationsPanel({
  onSessionExpired,
}: ConversationsPanelProps) {
  const [state, setState] = useState<LoadState>({ status: "loading" });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    // 1. CREATE A CANCELLABLE REQUEST
    const controller = new AbortController();

    async function loadConversations() {
      try {
        const response = await fetch("/api/conversations", {
          credentials: "same-origin",
          cache: "no-store",
          signal: controller.signal,
        });

        if (controller.signal.aborted) {
          return;
        }

        // 2. RETURN TO LOGIN IF THE SESSION IS NO LONGER VALID
        if (response.status === 401) {
          onSessionExpired();
          return;
        }

        if (!response.ok) {
          throw new Error("Conversation request failed");
        }

        // 3. STORE THE CONVERSATIONS IN THE ORDER GO RETURNS THEM
        const conversations: Conversation[] = await response.json();

        if (!controller.signal.aborted) {
          setState({ status: "success", conversations });
        }
      } catch {
        // Cancellation is expected when leaving this component.
        if (!controller.signal.aborted) {
          setState({
            status: "error",
            message: "Could not load conversations. Please try again.",
          });
        }
      }
    }

    void loadConversations();

    // 4. CANCEL WHEN UNMOUNTING OR STARTING ANOTHER ATTEMPT
    return () => controller.abort();
  }, [attempt, onSessionExpired]);

  function retry() {
    setState({ status: "loading" });
    setAttempt((previous) => previous + 1);
  }

  if (state.status === "loading") {
    return <p role="status">Loading conversations…</p>;
  }

  if (state.status === "error") {
    return (
      <div>
        <p role="alert">{state.message}</p>
        <button type="button" onClick={retry}>
          Try again
        </button>
      </div>
    );
  }

  return <ConversationList conversations={state.conversations} />;
}
