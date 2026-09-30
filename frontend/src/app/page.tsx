"use client";

import { useState, type FormEvent } from "react";

export default function Home() {
  // 1. STORE FORM VALUES AND REQUEST STATE
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [isLoggedIn, setIsLoggedIn] = useState(false);

  async function handleLogin(event: FormEvent<HTMLFormElement>) {
    // 2. HANDLE THE FORM WITHOUT RELOADING THE PAGE
    event.preventDefault();

    if (isSubmitting) {
      return;
    }

    setIsSubmitting(true);
    setError("");

    try {
      // 3. SUBMIT CREDENTIALS THROUGH THE NEXT.JS REWRITE
      const response = await fetch("/api/login", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        credentials: "same-origin",
        body: JSON.stringify({ username, password }),
      });

      // 4. TRANSLATE HTTP RESULTS INTO USER-FACING MESSAGES
      if (response.status === 401) {
        setError("Incorrect username or password.");
        return;
      }

      if (response.status === 400) {
        setError("Please check your username and password.");
        return;
      }

      if (!response.ok) {
        setError("Login is unavailable right now. Please try again.");
        return;
      }

      // Go returns 204 with no body, so do not call response.json().
      // The browser stores the session cookie from the response.
      setPassword("");
      setIsLoggedIn(true);
    } catch {
      // fetch throws for network failures, not ordinary HTTP errors.
      setError("Could not reach the server. Please try again.");
    } finally {
      // 5. RE-ENABLE THE FORM AFTER EVERY OUTCOME
      setIsSubmitting(false);
    }
  }

  // 6. DISPLAY A TEMPORARY SUCCESS VIEW
  if (isLoggedIn) {
    return (
      <main>
        <h1>Login successful</h1>
        <p>Your session has been created.</p>
      </main>
    );
  }

  // 7. RENDER THE LOGIN FORM
  return (
    <main>
      <h1>Chat login</h1>
      <p>Sign in with an account you created through the API.</p>

      <form onSubmit={handleLogin} aria-busy={isSubmitting}>
        <div>
          <label htmlFor="username">Username</label>
          <input
            id="username"
            name="username"
            type="text"
            autoComplete="username"
            required
            value={username}
            onChange={(event) => setUsername(event.target.value)}
            disabled={isSubmitting}
          />
        </div>

        <div>
          <label htmlFor="password">Password</label>
          <input
            id="password"
            name="password"
            type="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            disabled={isSubmitting}
          />
        </div>

        {error && <p role="alert">{error}</p>}

        <button type="submit" disabled={isSubmitting}>
          {isSubmitting ? "Signing in…" : "Sign in"}
        </button>
      </form>
    </main>
  );
}
