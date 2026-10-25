import { shallowRef } from "vue";

const DATABASE_NAME = "nas-tiktok-saved-credentials";
const DATABASE_VERSION = 1;
const STORE_NAME = "records";
const KEY_ID = "key";
const CREDENTIAL_ID = "credential";
const DISABLED_ID = "disabled";
const DISABLED_STORAGE_KEY = "nas-tiktok.credentials-disabled";
const REMEMBER_PREFERENCE_KEY = "nas-tiktok.remember-credentials";
const SAVED_LOGIN_KEY = "savedLogin";
const FORMAT_VERSION = 1;

type CredentialKeyRecord = { id: typeof KEY_ID; key: CryptoKey };
type CredentialRecord = {
  id: typeof CREDENTIAL_ID;
  version: number;
  account: string;
  iv: ArrayBuffer;
  ciphertext: ArrayBuffer;
};
type DisabledRecord = { id: typeof DISABLED_ID; disabled: true };
type StoredRecord = CredentialKeyRecord | CredentialRecord | DisabledRecord;

export type PersistResult =
  | { status: "saved"; notice?: string }
  | { status: "cancelled" }
  | { status: "failed"; notice: string };

export const credentialPersistenceNotice = shallowRef("");

let operationVersion = 0;
let rememberEnabled = false;
let memoryDisabled = false;
let writeQueue: Promise<unknown> = Promise.resolve();

export const credentialOperationVersion = () => operationVersion;

const serializeWrite = <T>(operation: () => Promise<T>): Promise<T> => {
  const result = writeQueue.then(operation, operation);
  writeQueue = result.then(() => undefined, () => undefined);
  return result;
};

const transactionComplete = (transaction: IDBTransaction) =>
  new Promise<void>((resolve, reject) => {
    transaction.oncomplete = () => resolve();
    transaction.onabort = () => reject(transaction.error ?? new Error("IndexedDB transaction aborted"));
    transaction.onerror = () => reject(transaction.error ?? new Error("IndexedDB transaction failed"));
  });

const requestValue = <T>(request: IDBRequest<T>) =>
  new Promise<T>((resolve, reject) => {
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error ?? new Error("IndexedDB request failed"));
  });

const openDatabase = () =>
  new Promise<IDBDatabase>((resolve, reject) => {
    let request: IDBOpenDBRequest;
    try {
      request = indexedDB.open(DATABASE_NAME, DATABASE_VERSION);
    } catch (error) {
      reject(error);
      return;
    }

    let settled = false;
    request.onupgradeneeded = () => {
      if (!request.result.objectStoreNames.contains(STORE_NAME)) {
        request.result.createObjectStore(STORE_NAME, { keyPath: "id" });
      }
    };
    request.onsuccess = () => {
      if (settled) {
        request.result.close();
        return;
      }
      settled = true;
      request.result.onversionchange = () => request.result.close();
      resolve(request.result);
    };
    request.onerror = () => {
      if (settled) return;
      settled = true;
      reject(request.error ?? new Error("IndexedDB open failed"));
    };
    request.onblocked = () => {
      if (settled) return;
      settled = true;
      reject(new Error("IndexedDB open blocked"));
    };
  });

const runWriteTransaction = async (
  database: IDBDatabase,
  write: (store: IDBObjectStore) => void
) => {
  const transaction = database.transaction(STORE_NAME, "readwrite");
  const completed = transactionComplete(transaction);
  try {
    write(transaction.objectStore(STORE_NAME));
  } catch (error) {
    try {
      transaction.abort();
    } catch {
      // The transaction may already be inactive.
    }
    await completed.catch(() => undefined);
    throw error;
  }
  await completed;
};

const readCredentialRecords = async (database: IDBDatabase) => {
  const transaction = database.transaction(STORE_NAME, "readonly");
  const completed = transactionComplete(transaction);
  try {
    const store = transaction.objectStore(STORE_NAME);
    const [keyRecord, credential, disabled] = await Promise.all([
      requestValue<CredentialKeyRecord | undefined>(store.get(KEY_ID)),
      requestValue<CredentialRecord | undefined>(store.get(CREDENTIAL_ID)),
      requestValue<DisabledRecord | undefined>(store.get(DISABLED_ID)),
    ]);
    await completed;
    return { keyRecord, credential, disabled };
  } catch (error) {
    await completed.catch(() => undefined);
    throw error;
  }
};

const writeCredentialsAndDeleteTombstone = (
  database: IDBDatabase,
  key: CryptoKey,
  account: string,
  iv: ArrayBuffer,
  ciphertext: ArrayBuffer
) =>
  runWriteTransaction(database, (store) => {
    store.put({ id: KEY_ID, key } satisfies CredentialKeyRecord);
    store.put({
      id: CREDENTIAL_ID,
      version: FORMAT_VERSION,
      account,
      iv,
      ciphertext,
    } satisfies CredentialRecord);
    store.delete(DISABLED_ID);
  });

const readDisabledStorageMarker = () => {
  try {
    return localStorage.getItem(DISABLED_STORAGE_KEY) === "1";
  } catch {
    return null;
  }
};

const readRememberPreference = (): boolean | null => {
  try {
    const preference = localStorage.getItem(REMEMBER_PREFERENCE_KEY);
    if (preference === "1") return true;
    if (preference === "0") return false;
    return null;
  } catch {
    return null;
  }
};

export const getCredentialPreference = () =>
  isHttpsOrigin() ? readRememberPreference() : null;

const writeRememberPreference = (enabled: boolean) => {
  try {
    localStorage.setItem(REMEMBER_PREFERENCE_KEY, enabled ? "1" : "0");
    return true;
  } catch {
    return false;
  }
};

export const isHttpsOrigin = () =>
  typeof window !== "undefined" && window.location.protocol === "https:";

const hasSavedCredentialSupport = () =>
  isHttpsOrigin() &&
  typeof window !== "undefined" &&
  window.isSecureContext &&
  typeof indexedDB !== "undefined" &&
  typeof crypto !== "undefined" &&
  typeof crypto.getRandomValues === "function" &&
  typeof crypto.subtle?.generateKey === "function" &&
  typeof crypto.subtle?.encrypt === "function" &&
  typeof crypto.subtle?.decrypt === "function";

const isValidKey = (value: unknown): value is CryptoKey => {
  if (typeof CryptoKey === "undefined" || !(value instanceof CryptoKey)) return false;
  const algorithm = value.algorithm;
  if (algorithm.name !== "AES-GCM" || !("length" in algorithm)) return false;
  return (
    algorithm.length === 256 &&
    value.type === "secret" &&
    value.extractable === false &&
    value.usages.length === 2 &&
    value.usages.includes("encrypt") &&
    value.usages.includes("decrypt")
  );
};

export const setCredentialPreference = (enabled: boolean) => {
  if (!isHttpsOrigin()) {
    return { version: operationVersion, markerWritten: false, preferenceWritten: false };
  }
  const version = ++operationVersion;
  rememberEnabled = enabled;
  const preferenceWritten = writeRememberPreference(enabled);
  if (enabled) return { version, markerWritten: false, preferenceWritten };

  memoryDisabled = true;
  let markerWritten = false;
  try {
    localStorage.setItem(DISABLED_STORAGE_KEY, "1");
    markerWritten = true;
  } catch {
    // The IndexedDB tombstone is attempted separately by clearSavedCredentials.
  }
  return { version, markerWritten, preferenceWritten };
};

export const loadSavedCredentials = async () => {
  if (!isHttpsOrigin()) {
    return { status: "unavailable" as const };
  }
  const startingVersion = operationVersion;
  if (readRememberPreference() === false) {
    return { status: "disabled" as const };
  }
  const disabledMarker = readDisabledStorageMarker();
  if (memoryDisabled || disabledMarker === true) {
    return { status: "disabled" as const };
  }
  if (disabledMarker === null) return { status: "unavailable" as const };
  if (!hasSavedCredentialSupport()) return { status: "unavailable" as const };

  let database: IDBDatabase | undefined;
  try {
    database = await openDatabase();
    const { keyRecord, credential, disabled } = await readCredentialRecords(database);

    if (disabled?.disabled) {
      memoryDisabled = true;
      return { status: "disabled" as const };
    }
    if (startingVersion !== operationVersion || memoryDisabled) {
      return { status: "disabled" as const };
    }
    if (!keyRecord && !credential) return { status: "empty" as const };
    if (
      !keyRecord ||
      !credential ||
      !isValidKey(keyRecord.key) ||
      credential.version !== FORMAT_VERSION ||
      typeof credential.account !== "string" ||
      !(credential.iv instanceof ArrayBuffer) ||
      credential.iv.byteLength !== 12 ||
      !(credential.ciphertext instanceof ArrayBuffer)
    ) return { status: "unavailable" as const };

    try {
      const plaintext = await crypto.subtle.decrypt(
        { name: "AES-GCM", iv: new Uint8Array(credential.iv) },
        keyRecord.key,
        credential.ciphertext
      );
      const markerAfterDecrypt = readDisabledStorageMarker();
      if (markerAfterDecrypt === null) return { status: "unavailable" as const };
      if (
        startingVersion !== operationVersion ||
        memoryDisabled ||
        markerAfterDecrypt
      ) return { status: "disabled" as const };
      return {
        status: "loaded" as const,
        account: credential.account,
        password: new TextDecoder().decode(plaintext),
      };
    } catch {
      // A read/decrypt failure does not delete existing key or ciphertext.
      return { status: "unavailable" as const };
    }
  } catch {
    // Open, transaction, request, and structured-clone errors all fail closed.
    return { status: "unavailable" as const };
  } finally {
    database?.close();
  }
};

export const clearSavedCredentials = (
  expectedVersion: number,
  localMarkerWritten: boolean
) =>
  serializeWrite(async () => {
    let savedLoginCleared = false;
    let tombstoneWritten = false;
    let credentialsCleared = false;
    let database: IDBDatabase | undefined;

    // This is independent of every IndexedDB operation; an IDB failure must
    // not skip the legacy account-only record removal.
    try {
      localStorage.removeItem(SAVED_LOGIN_KEY);
      savedLoginCleared = true;
    } catch {
      // Report the incomplete cleanup below.
    }

    try {
      database = await openDatabase();
    } catch {
      // Still return the localStorage result and marker state accurately.
    }

    if (database) {
      try {
        await runWriteTransaction(database, (store) => {
          store.put({ id: DISABLED_ID, disabled: true } satisfies DisabledRecord);
        });
        tombstoneWritten = true;
      } catch {
        // Attempt the delete transaction even if persisting the tombstone failed.
      }

      try {
        await runWriteTransaction(database, (store) => {
          store.delete(KEY_ID);
          store.delete(CREDENTIAL_ID);
        });
        credentialsCleared = true;
      } catch {
        // The already-written tombstone or localStorage marker still blocks fill.
      } finally {
        database.close();
      }
    }

    const futureAutofillBlocked =
      localMarkerWritten || tombstoneWritten || credentialsCleared;
    return {
      complete: savedLoginCleared && credentialsCleared,
      savedLoginCleared,
      credentialsCleared,
      futureAutofillBlocked,
      versionIsCurrent: expectedVersion === operationVersion,
    };
  });

export const persistSuccessfulLogin = (
  account: string,
  password: string,
  expectedVersion: number
): Promise<PersistResult> =>
  serializeWrite(async () => {
    if (expectedVersion !== operationVersion || !rememberEnabled) {
      return { status: "cancelled" };
    }

    let accountNotice: string | undefined;
    try {
      if (!hasSavedCredentialSupport()) {
        return {
          status: "failed",
          notice: "登录成功，但当前浏览器不能安全保存密码。",
        };
      }
      if (!writeRememberPreference(true)) {
        return {
          status: "failed",
          notice: "无法保存“记住账号和密码”的设置；本机登录信息未保存。",
        };
      }
      const key = await crypto.subtle.generateKey(
        { name: "AES-GCM", length: 256 },
        false,
        ["encrypt", "decrypt"]
      );
      const iv = crypto.getRandomValues(new Uint8Array(12));
      const ciphertext = await crypto.subtle.encrypt(
        { name: "AES-GCM", iv },
        key,
        new TextEncoder().encode(password)
      );

      if (expectedVersion !== operationVersion || !rememberEnabled) {
        return { status: "cancelled" };
      }

      const database = await openDatabase();
      try {
        // A prior disable is lifted in the same commit as the new credentials.
        await writeCredentialsAndDeleteTombstone(
          database,
          key,
          account,
          iv.buffer.slice(iv.byteOffset, iv.byteOffset + iv.byteLength),
          ciphertext
        );
      } finally {
        database.close();
      }

      if (expectedVersion !== operationVersion || !rememberEnabled) {
        return { status: "cancelled" };
      }

      try {
        localStorage.setItem(SAVED_LOGIN_KEY, JSON.stringify({ account }));
      } catch {
        accountNotice = "账号偏好无法保存在浏览器中。";
      }

      try {
        localStorage.removeItem(DISABLED_STORAGE_KEY);
      } catch {
        memoryDisabled = true;
        return {
          status: "saved",
          notice: "密码已加密保存，但浏览器停用标记无法清除，自动回填可能仍被禁用。",
        };
      }

      memoryDisabled = false;
      return { status: "saved", notice: accountNotice };
    } catch {
      return {
        status: "failed",
        notice: "登录成功，但本机未能安全保存账号密码。",
      };
    }
  });
