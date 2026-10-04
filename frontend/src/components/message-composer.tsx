"use client";

import { useEffect, useRef, useState, type FormEvent } from "react";
import type { Message } from "@/types/message";

type MessageComposerProps = {
  conversationId: number;
  onMessageSent: (message: Message) => void;
  onSessionExpired: () => void;
  onConversationUnavailable: () => void;
};

export default function MessageComposer({
  conversationId,
  onMessageSent,
  onSessionExpired,
  onConversationUnavailable,
}: MessageComposerProps) {
  // 1. STORE THE DRAFT AND REQUEST STATE
  const [body, setBody] = useState("");
  const [error, setError] = useState("");
  const [isSending, setIsSending] = useState(false);

  const activeRequest = useRef<AbortController | null>(null);

  // Cancel the browser request when this composer is removed.
  useEffect(() => {
    return () => activeRequest.current?.abort();
  }, []);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    // 2. PREVENT OVERLAPPING SUBMISSIONS
    if (activeRequest.current !== null) {
      return;
    }

    setError("");

    if (body.trim() === "") {
      setError("Enter a message before sending.");
      return;
    }

    if (Array.from(body).length > 4000) {
      setError("Messages must contain no more than 4,000 characters.");
      return;
    }

    const controller = new AbortController();
    activeRequest.current = controller;
    setIsSending(true);

    try {
      // 3. SEND ONLY THE MESSAGE BODY
      // The authenticated session determines the sender.
      const response = await fetch(
        `/api/conversations/${conversationId}/messages`,
        {
          method: "POST",
          credentials: "same-origin",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({ body }),
          signal: controller.signal,
        },
      );

      if (controller.signal.aborted) {
        return;
      }

      // 4. HANDLE AUTHENTICATION, ACCESS, AND VALIDATION FAILURES
      if (response.status === 401) {
        onSessionExpired();
        return;
      }

      if (response.status === 404) {
        onConversationUnavailable();
        return;
      }

      if (response.status === 400) {
        setError("The server rejected this message. Please check its content.");
        return;
      }

      if (response.status === 403) {
        setError("The request was forbidden. Your draft has been kept.");
        return;
      }

      if (response.status !== 201) {
        setError(
          "Could not confirm sending. Your draft is kept; check the history before retrying.",
        );
        return;
      }

      const message: Message = await response.json();

      if (controller.signal.aborted) {
        return;
      }

      // 5. ADD THE SERVER-CONFIRMED MESSAGE AND CLEAR THE DRAFT
      onMessageSent(message);
      setBody("");
    } catch {
      if (!controller.signal.aborted) {
        setError(
          "Could not confirm sending. Your draft is kept; check the history before retrying.",
        );
      }
    } finally {
      if (activeRequest.current === controller) {
        activeRequest.current = null;
      }

      if (!controller.signal.aborted) {
        setIsSending(false);
      }
    }
  }

  // 6. KEEP THE DRAFT VISIBLE WHILE THE REQUEST IS PENDING
  return (
    <form onSubmit={handleSubmit} aria-busy={isSending}>
      <label htmlFor="message-body">Message</label>

      <textarea
        id="message-body"
        name="body"
        rows={3}
        value={body}
        onChange={(event) => setBody(event.target.value)}
        disabled={isSending}
        aria-describedby={error ? "message-error" : undefined}
      />

      {error && (
        <p id="message-error" role="alert">
          {error}
        </p>
      )}

      <button type="submit" disabled={isSending}>
        {isSending ? "Sending…" : "Send"}
      </button>
    </form>
  );
}
