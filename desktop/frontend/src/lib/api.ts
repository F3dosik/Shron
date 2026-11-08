// Обёртки над привязками Wails: ошибки из Go приходят текстом JSON {code, message}
// (см. desktop/backend/errors.go) и превращаются в AppError.
import * as Go from '../../wailsjs/go/backend/App.js'
import { backend } from '../../wailsjs/go/models'

export type State = backend.State
export type SecretSummary = backend.SecretSummary
export type Secret = backend.Secret
export type ChosenFile = backend.ChosenFile
export type SecretInput = backend.SecretInput
export type UserRow = backend.UserRow
export type TemporaryUser = backend.TemporaryUser
export type Generated = backend.Generated
export type GeneratorOptions = backend.GeneratorOptions
export type ImportResult = backend.ImportResult

/** Ошибка вызова Go-метода: code — категория (WRONG_PASSWORD, LOCKED, ...). */
export class AppError extends Error {
  constructor(public code: string, message: string) {
    super(message)
  }
}

function toAppError(e: unknown): AppError {
  const text = typeof e === 'string' ? e : e instanceof Error ? e.message : String(e)
  try {
    const parsed = JSON.parse(text)
    if (parsed && typeof parsed.code === 'string') {
      return new AppError(parsed.code, parsed.message ?? text)
    }
  } catch {
    // не JSON — ошибка самой оболочки
  }
  return new AppError('INTERNAL', text)
}

async function call<T>(promise: Promise<T>): Promise<T> {
  try {
    return await promise
  } catch (e) {
    throw toAppError(e)
  }
}

/** Ошибки, при которых нужно вернуться к экрану разблокировки или входа. */
export const lockedCodes = new Set(['LOCKED', 'SESSION_EXPIRED', 'NOT_SIGNED_IN'])

export const api = {
  getState: () => call(Go.GetState()),
  chooseCACert: () => call(Go.ChooseCACert()),
  checkServer: (address: string, caPEM: string, useSavedCA: boolean) => call(Go.CheckServer(address, caPEM, useSavedCA)),
  saveSettings: (address: string, caPEM: string, useSavedCA: boolean, autoLock: number) =>
    call(Go.SaveSettings(address, caPEM, useSavedCA, autoLock)),
  register: (login: string, password: string) => call(Go.Register(login, password)),
  signIn: (login: string, password: string) => call(Go.SignIn(login, password)),
  completePasswordChange: (temp: string, next: string) => call(Go.CompletePasswordChange(temp, next)),
  unlock: (password: string) => call(Go.Unlock(password)),
  acceptKdfChange: (login: string) => call(Go.AcceptKDFChange(login)),
  lock: () => call(Go.Lock()),
  retryServer: () => call(Go.RetryServer()),
  changePassword: (oldPw: string, newPw: string, kdfTime: number, kdfMemoryMiB: number) =>
    call(Go.ChangePassword(oldPw, newPw, kdfTime, kdfMemoryMiB)),
  logout: (all: boolean) => call(Go.Logout(all)),
  deleteAccount: (password: string) => call(Go.DeleteAccount(password)),
  copy: (value: string) => call(Go.CopyToClipboard(value)),
  listSecrets: () => call(Go.ListSecrets()),
  getSecret: (name: string, type: string) => call(Go.GetSecret(name, type)),
  saveSecret: (input: SecretInput, update: boolean) => call(Go.SaveSecret(input, update)),
  deleteSecret: (name: string, type: string) => call(Go.DeleteSecret(name, type)),
  chooseFile: () => call(Go.ChooseFile()),
  exportFile: (name: string) => call(Go.ExportFile(name)),

  exportVault: (password: string) => call(Go.ExportVault(password)),
  chooseExportFile: () => call(Go.ChooseExportFile()),
  importVault: (path: string, password: string, overwrite: boolean) => call(Go.ImportVault(path, password, overwrite)),

  generatePassword: (opts: GeneratorOptions) => call(Go.GeneratePassword(opts)),
  generatePassphrase: (words: number) => call(Go.GeneratePassphrase(words)),

  adminListUsers: () => call(Go.AdminListUsers()),
  adminCreateTemporaryUser: (login: string) => call(Go.AdminCreateTemporaryUser(login)),
  adminRevokeSessions: (login: string) => call(Go.AdminRevokeSessions(login)),
  adminDeleteUser: (login: string) => call(Go.AdminDeleteUser(login)),
}

/** Текст ошибки для показа пользователю. */
export function errorText(e: unknown): string {
  return e instanceof AppError ? e.message : String(e)
}
