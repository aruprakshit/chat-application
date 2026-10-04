"use client";

import { useState, type FormEvent } from "react";

export default function MessageComposer() {
  // 1. STORE THE DRAFT AND VALIDATION FEEDBACK
  const [body, setBody] = useState("");
  const [error, setError] = useState("");

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");

    // 2. REJECT WHITESPACE-ONLY MESSAGES WITHOUT CHANGING THE DRAFT
    if (body.trim() === "") {
      setError("Enter a message before sending.");
      return;
    }

    // 3. COUNT UNICODE CODE POINTS, MATCHING THE GO HANDLER
    if (Array.from(body).length > 4000) {
      setError("Messages must contain no more than 4,000 characters.");
      return;
    }

    // 4. TEMPORARY CHECKPOINT: NO REQUEST IS SENT YET
    setError("Message is valid. Sending is not connected yet.");
  }

  return (
    <form onSubmit={handleSubmit}>
      <label htmlFor="message-body">Message</label>

      <textarea
        id="message-body"
        name="body"
        rows={3}
        value={body}
        onChange={(event) => setBody(event.target.value)}
        aria-invalid={Boolean(error)}
        aria-describedby={error ? "message-error" : undefined}
      />

      {error && (
        <p id="message-error" role="alert">
          {error}
        </p>
      )}

      <button type="submit">Send</button>
    </form>
  );
}
