import {
  SESSION_STORAGE_KEY,
  clearStoredSessionId,
  getStoredSessionId,
  setStoredSessionId,
} from "./session-storage";

function createStorageStub() {
  const values = new Map();

  return {
    getItem(key) {
      return values.has(key) ? values.get(key) : null;
    },
    setItem(key, value) {
      values.set(key, value);
    },
    removeItem(key) {
      values.delete(key);
    },
  };
}

describe("session storage", () => {
  it("stores and reads session IDs", () => {
    const storage = createStorageStub();

    setStoredSessionId("abc-123", storage);

    expect(storage.getItem(SESSION_STORAGE_KEY)).toBe("abc-123");
    expect(getStoredSessionId(storage)).toBe("abc-123");
  });

  it("normalizes empty values to null", () => {
    const storage = createStorageStub();

    storage.setItem(SESSION_STORAGE_KEY, "   ");

    expect(getStoredSessionId(storage)).toBeNull();
  });

  it("clears stored session IDs", () => {
    const storage = createStorageStub();

    storage.setItem(SESSION_STORAGE_KEY, "abc-123");
    clearStoredSessionId(storage);

    expect(getStoredSessionId(storage)).toBeNull();
  });

  it("throws for empty session IDs", () => {
    const storage = createStorageStub();

    expect(() => setStoredSessionId("", storage)).toThrow("sessionId must be a non-empty string");
  });
});
