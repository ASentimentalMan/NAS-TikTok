// @vitest-environment jsdom
// @vitest-environment-options {"url":"https://localhost/"}

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

class TestCryptoKey {
  readonly type = "secret";
  readonly algorithm = { name: "AES-GCM", length: 256 };
  readonly extractable = false;
  readonly usages: KeyUsage[] = ["encrypt", "decrypt"];
}

const makeIndexedDB = () => {
  const records = new Map<string, unknown>();
  let openThrows = false;
  let transactionThrows = false;
  let transactionIndex = 0;
  const failedTransactionIndexes = new Set<number>();

  const database = {
    objectStoreNames: { contains: () => true },
    close: vi.fn(),
    transaction: () => {
      if (transactionThrows) throw new Error("transaction unavailable");
      transactionIndex += 1;
      const currentTransaction = transactionIndex;
      const operations: (() => void)[] = [];
      const transaction: any = { error: null, oncomplete: null, onabort: null, onerror: null };
      transaction.objectStore = () => ({
        get: (id: string) => {
          const request: any = { result: undefined, error: null, onsuccess: null, onerror: null };
          queueMicrotask(() => {
            request.result = records.get(id);
            request.onsuccess?.({ target: request });
          });
          return request;
        },
        put: (value: { id: string }) => operations.push(() => records.set(value.id, value)),
        delete: (id: string) => operations.push(() => records.delete(id)),
      });
      queueMicrotask(() => {
        if (failedTransactionIndexes.has(currentTransaction)) {
          failedTransactionIndexes.delete(currentTransaction);
          transaction.error = new Error("transaction aborted");
          transaction.onabort?.({ target: transaction });
          return;
        }
        operations.forEach((operation) => operation());
        transaction.oncomplete?.({ target: transaction });
      });
      return transaction;
    },
  };

  const indexedDB: any = {
    open: () => {
      if (openThrows) throw new Error("database unavailable");
      const request: any = { result: undefined, error: null, onupgradeneeded: null, onsuccess: null, onerror: null, onblocked: null };
      queueMicrotask(() => {
        request.result = database;
        request.onsuccess?.({ target: request });
      });
      return request;
    },
  };

  return {
    indexedDB,
    records,
    failOpen: () => { openThrows = true; },
    recoverOpen: () => { openThrows = false; },
    failTransactionAt: (index: number) => { failedTransactionIndexes.add(index); },
    failTransactionCreation: () => { transactionThrows = true; },
    recoverTransactionCreation: () => { transactionThrows = false; },
  };
};

const flushMicrotasks = async () => {
  for (let index = 0; index < 8; index += 1) await Promise.resolve();
};

describe("encrypted saved credentials", () => {
  let fakeDb: ReturnType<typeof makeIndexedDB>;
  let storage: typeof import("@/utils/saved-credentials");
  let secureContextDescriptor: PropertyDescriptor | undefined;
  let subtle: {
    generateKey: ReturnType<typeof vi.fn>;
    encrypt: ReturnType<typeof vi.fn>;
    decrypt: ReturnType<typeof vi.fn>;
  };

  beforeEach(async () => {
    vi.resetModules();
    localStorage.clear();
    fakeDb = makeIndexedDB();
    subtle = {
      generateKey: vi.fn().mockResolvedValue(new TestCryptoKey()),
      encrypt: vi.fn().mockResolvedValue(new Uint8Array([0xa1, 0xb2, 0xc3]).buffer),
      decrypt: vi.fn().mockResolvedValue(new TextEncoder().encode("test-password").buffer),
    };
    vi.stubGlobal("indexedDB", fakeDb.indexedDB);
    vi.stubGlobal("CryptoKey", TestCryptoKey);
    vi.stubGlobal("crypto", {
      getRandomValues: (value: Uint8Array) => {
        value.forEach((_item, index) => { value[index] = index + 1; });
        return value;
      },
      subtle,
    });
    secureContextDescriptor = Object.getOwnPropertyDescriptor(window, "isSecureContext");
    Object.defineProperty(window, "isSecureContext", { configurable: true, value: true });
    storage = await import("@/utils/saved-credentials");
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    if (secureContextDescriptor) {
      Object.defineProperty(window, "isSecureContext", secureContextDescriptor);
    } else {
      Reflect.deleteProperty(window, "isSecureContext");
    }
    localStorage.clear();
  });

  it("stores an AES-GCM ciphertext and decrypts it on a later read", async () => {
    const preference = storage.setCredentialPreference(true);
    const result = await storage.persistSuccessfulLogin("saved-user", "test-password", preference.version);

    expect(result).toMatchObject({ status: "saved" });
    expect(subtle.generateKey).toHaveBeenCalledWith(
      { name: "AES-GCM", length: 256 },
      false,
      ["encrypt", "decrypt"]
    );
    const [algorithm, key, plaintext] = subtle.encrypt.mock.calls[0];
    expect(algorithm.name).toBe("AES-GCM");
    expect(algorithm.iv).toBeInstanceOf(Uint8Array);
    expect(algorithm.iv).toHaveLength(12);
    expect(key).toBeInstanceOf(TestCryptoKey);
    expect(plaintext).toEqual(new TextEncoder().encode("test-password"));

    const credential = fakeDb.records.get("credential") as {
      account: string;
      iv: ArrayBuffer;
      ciphertext: ArrayBuffer;
    };
    expect(credential.account).toBe("saved-user");
    expect(credential.iv.byteLength).toBe(12);
    expect(new TextDecoder().decode(credential.ciphertext)).not.toContain("test-password");
    expect(localStorage.getItem("savedLogin")).not.toContain("test-password");

    vi.resetModules();
    storage = await import("@/utils/saved-credentials");
    await expect(storage.loadSavedCredentials()).resolves.toMatchObject({
      status: "loaded",
      account: "saved-user",
      password: "test-password",
    });
  });

  it("keeps the remember preference across a module reload independently of credentials", async () => {
    const enabled = storage.setCredentialPreference(true);

    expect(enabled.preferenceWritten).toBe(true);
    expect(storage.getCredentialPreference()).toBe(true);

    vi.resetModules();
    storage = await import("@/utils/saved-credentials");
    expect(storage.getCredentialPreference()).toBe(true);
    await expect(storage.loadSavedCredentials()).resolves.toMatchObject({ status: "empty" });

    const disabled = storage.setCredentialPreference(false);
    expect(disabled.preferenceWritten).toBe(true);
    vi.resetModules();
    storage = await import("@/utils/saved-credentials");
    expect(storage.getCredentialPreference()).toBe(false);
    await expect(storage.loadSavedCredentials()).resolves.toMatchObject({ status: "disabled" });
  });

  it("deletes account storage even when IndexedDB open fails", async () => {
    localStorage.setItem("savedLogin", JSON.stringify({ account: "user" }));
    const preference = storage.setCredentialPreference(false);
    fakeDb.failOpen();

    const result = await storage.clearSavedCredentials(preference.version, preference.markerWritten);

    expect(localStorage.getItem("savedLogin")).toBeNull();
    expect(result.complete).toBe(false);
    expect(result.futureAutofillBlocked).toBe(true);
    await expect(storage.loadSavedCredentials()).resolves.toMatchObject({ status: "disabled" });
  });

  it("does not claim complete cleanup when localStorage removal fails", async () => {
    const preference = storage.setCredentialPreference(false);
    const originalRemoveItem = Storage.prototype.removeItem;
    const removeItem = vi.spyOn(Storage.prototype, "removeItem").mockImplementation(function (this: Storage, key: string) {
      if (key === "savedLogin") throw new DOMException("storage denied", "SecurityError");
      return originalRemoveItem.call(this, key);
    });

    const result = await storage.clearSavedCredentials(preference.version, preference.markerWritten);

    expect(removeItem).toHaveBeenCalledWith("savedLogin");
    expect(result.complete).toBe(false);
    expect(result.savedLoginCleared).toBe(false);
    expect(result.credentialsCleared).toBe(true);
    expect(result.futureAutofillBlocked).toBe(true);
    removeItem.mockRestore();
  });

  it("retains the durable tombstone when credential deletion fails", async () => {
    localStorage.setItem("savedLogin", JSON.stringify({ account: "legacy-user" }));
    const preference = storage.setCredentialPreference(false);
    fakeDb.failTransactionAt(2); // Tombstone commits; credential deletion transaction fails.
    const result = await storage.clearSavedCredentials(preference.version, preference.markerWritten);

    expect(localStorage.getItem("savedLogin")).toBeNull();
    expect(result.complete).toBe(false);
    expect(result.savedLoginCleared).toBe(true);
    expect(result.credentialsCleared).toBe(false);
    expect(fakeDb.records.get("disabled")).toMatchObject({ disabled: true });
    expect(result.futureAutofillBlocked).toBe(true);
    await expect(storage.loadSavedCredentials()).resolves.toMatchObject({ status: "disabled" });
  });

  it("fails closed in memory and reports when both persistent stores cannot write", async () => {
    const originalSetItem = Storage.prototype.setItem;
    const setItem = vi.spyOn(Storage.prototype, "setItem").mockImplementation(function (this: Storage, key: string, value: string) {
      if (key === "nas-tiktok.credentials-disabled") {
        throw new DOMException("storage denied", "QuotaExceededError");
      }
      return originalSetItem.call(this, key, value);
    });
    fakeDb.failOpen();
    const preference = storage.setCredentialPreference(false);
    const result = await storage.clearSavedCredentials(preference.version, preference.markerWritten);

    expect(result.complete).toBe(false);
    expect(result.futureAutofillBlocked).toBe(false);
    await expect(storage.loadSavedCredentials()).resolves.toMatchObject({ status: "disabled" });
    setItem.mockRestore();
  });

  it("catches IndexedDB transaction creation failures without unhandled rejection", async () => {
    fakeDb.failTransactionCreation();
    await expect(storage.loadSavedCredentials()).resolves.toMatchObject({ status: "unavailable" });
    const preference = storage.setCredentialPreference(false);
    await expect(storage.clearSavedCredentials(preference.version, preference.markerWritten))
      .resolves.toMatchObject({ complete: false });
    await flushMicrotasks();
  });

  it("does not let a decrypt started before cancellation return credentials", async () => {
    let resolveDecrypt!: (value: ArrayBuffer) => void;
    const preference = storage.setCredentialPreference(true);
    await storage.persistSuccessfulLogin("saved-user", "test-password", preference.version);
    subtle.decrypt.mockImplementationOnce(
      () => new Promise((resolve) => { resolveDecrypt = resolve; })
    );

    const pendingRead = storage.loadSavedCredentials();
    await flushMicrotasks();
    const cancelled = storage.setCredentialPreference(false);
    resolveDecrypt(new TextEncoder().encode("test-password").buffer);
    await expect(pendingRead).resolves.toMatchObject({ status: "disabled" });
    await storage.clearSavedCredentials(cancelled.version, cancelled.markerWritten);
  });

  it("does not persist an encryption that was in flight when remember was cancelled", async () => {
    let resolveEncrypt!: (value: ArrayBuffer) => void;
    const preference = storage.setCredentialPreference(true);
    subtle.encrypt.mockImplementationOnce(
      () => new Promise((resolve) => { resolveEncrypt = resolve; })
    );

    const pendingSave = storage.persistSuccessfulLogin("saved-user", "test-password", preference.version);
    await flushMicrotasks();
    const cancelled = storage.setCredentialPreference(false);
    const pendingClear = storage.clearSavedCredentials(cancelled.version, cancelled.markerWritten);
    resolveEncrypt(new Uint8Array([0xa1, 0xb2, 0xc3]).buffer);

    await expect(pendingSave).resolves.toEqual({ status: "cancelled" });
    await pendingClear;
    expect(fakeDb.records.has("credential")).toBe(false);
    expect(fakeDb.records.has("key")).toBe(false);
  });

  it("does not autofill when checking the disable marker fails", async () => {
    const preference = storage.setCredentialPreference(true);
    await storage.persistSuccessfulLogin("saved-user", "test-password", preference.version);
    const originalGetItem = Storage.prototype.getItem;
    const getItem = vi.spyOn(Storage.prototype, "getItem").mockImplementation(function (this: Storage, key: string) {
      if (key === "nas-tiktok.credentials-disabled") {
        throw new DOMException("storage denied", "SecurityError");
      }
      return originalGetItem.call(this, key);
    });

    await expect(storage.loadSavedCredentials()).resolves.toMatchObject({ status: "unavailable" });
    getItem.mockRestore();
  });
});
