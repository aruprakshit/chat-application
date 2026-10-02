"use client";

import { useEffect, useCallback, useState, type FormEvent } from "react";
import ConversationsPanel from "@/components/conversations-panel";

type User = {
  id: number;
  username: string;
};

export default function Home() {
  // 1. STORE FORM VALUES AND REQUEST STATE
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [user, setUser] = useState<User | null>(null);
  const [isCheckingSession, setIsCheckingSession] = useState(true);
  const [sessionError, setSessionError] = useState("");
  const [isLoggingOut, setIsLoggingOut] = useState(false);
  const [logoutError, setLogoutError] = useState("");

  // Clear the authenticated view when an API reports an expired session.
  const handleSessionExpired = useCallback(() => {
    setUser(null);
    setPassword("");
    setError("Your session has expired. Please sign in again.");
  }, []);

  // RESTORE THE SESSION WHEN THIS COMPONENT MOUNTS
  useEffect(() => {
    const controller = new AbortController();

    async function restoreSession() {
      try {
        const response = await fetch("/api/me", {
          credentials: "same-origin",
          cache: "no-store",
          signal: controller.signal,
        });

        // A missing or expired session is an ordinary logged-out state.
        if (response.status === 401) {
          return;
        }

        if (!response.ok) {
          throw new Error("Session check failed");
        }

        const currentUser: User = await response.json();

        if (!controller.signal.aborted) {
          setUser(currentUser);
        }
      } catch {
        if (!controller.signal.aborted) {
          setSessionError(
            "We couldn't check your session. Please reload to try again.",
          );
        }
      } finally {
        if (!controller.signal.aborted) {
          setIsCheckingSession(false);
        }
      }
    }

    void restoreSession();

    // Cancel this request if the component is removed.
    return () => controller.abort();
  }, []);

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

      // The login response has no body. Fetch the verified user separately.
      const meResponse = await fetch("/api/me", {
        credentials: "same-origin",
        cache: "no-store",
      });

      if (!meResponse.ok) {
        setError(
          "Login succeeded, but we couldn't load your account. Please reload.",
        );
        return;
      }

      const currentUser: User = await meResponse.json();
      setUser(currentUser);
    } catch {
      // fetch throws for network failures, not ordinary HTTP errors.
      setError("Could not reach the server. Please try again.");
    } finally {
      // 5. RE-ENABLE THE FORM AFTER EVERY OUTCOME
      setIsSubmitting(false);
    }
  }

  async function handleLogout() {
    // 1. PREVENT REPEATED SUBMISSIONS
    if (isLoggingOut) {
      return;
    }

    setIsLoggingOut(true);
    setLogoutError("");

    try {
      // 2. ASK GO TO REVOKE THE SESSION AND CLEAR THE COOKIE
      const response = await fetch("/api/logout", {
        method: "POST",
        credentials: "same-origin",
      });

      if (!response.ok) {
        setLogoutError("Could not sign out. Please try again.");
        return;
      }

      // 3. CLEAR LOCAL STATE AFTER THE SERVER CONFIRMS SUCCESS
      // The response has no JSON body.
      setUser(null);
      setUsername("");
      setPassword("");
      setError("");
    } catch {
      setLogoutError("Could not reach the server. Please try again.");
    } finally {
      setIsLoggingOut(false);
    }
  }

  if (isCheckingSession) {
    return (
      <main>
        <p role="status">Checking your session…</p>
      </main>
    );
  }

  if (sessionError) {
    return (
      <main>
        <h1>Unable to check session</h1>
        <p role="alert">{sessionError}</p>
        <button onClick={() => window.location.reload()}>Try again</button>
      </main>
    );
  }

  if (user) {
    return (
      <main>
        <h1>Welcome, {user.username}</h1>
        <section aria-labelledby="conversations-heading">
          <h2 id="conversations-heading">Your conversations</h2>
          <ConversationsPanel onSessionExpired={handleSessionExpired} />
        </section>
        <p>You are signed in.</p>

        {logoutError && <p role="alert">{logoutError}</p>}

        <button type="button" onClick={handleLogout} disabled={isLoggingOut}>
          {isLoggingOut ? "Signing out…" : "Sign out"}
        </button>
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
