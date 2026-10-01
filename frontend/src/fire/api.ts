import type { ActionValue } from "./actions";

const API_BASE_URL = import.meta.env.VITE_FIRE_API_BASE_URL;
const TIMEOUT_MS = 60_000;

export type StampedImage = {
  employeeNumber: string;
  /** スタンプを押した画像の Object URL */
  imageUrl: string;
};

/** 利用者に見せるエラー。 */
export class StampError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "StampError";
  }
}

/** スタンプの 2 行目に使う、端末の現在日時（YYYY/MM/DD HH:mm）。 */
export function clientDateTime(d = new Date()): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}/${pad(d.getMonth() + 1)}/${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/**
 * 画像をサーバー C へ送り、スタンプを押した画像を受け取る。
 * メタデータで社員を照合するため、画像は加工せずそのまま送る。
 */
export async function stampImage(
  action: ActionValue,
  image: File,
): Promise<StampedImage> {
  const body = new FormData();
  body.append("action", action);
  body.append("client_datetime", clientDateTime());
  body.append("image", image, image.name);

  let res: Response;
  try {
    res = await fetch(`${API_BASE_URL}/api/stamps`, {
      method: "POST",
      body,
      signal: AbortSignal.timeout(TIMEOUT_MS),
    });
  } catch {
    throw new StampError(
      "サーバーに接続できませんでした。時間をおいて再度お試しください。",
    );
  }

  if (!res.ok) {
    const data = (await res.json().catch(() => null)) as {
      error?: string;
    } | null;
    throw new StampError(
      data?.error ?? `処理に失敗しました（エラーコード ${res.status}）。`,
    );
  }

  const employeeNumber = res.headers.get("X-Employee-Number") ?? "";
  const blob = await res.blob();
  // 完了画面から戻るときに解放する。
  return { employeeNumber, imageUrl: URL.createObjectURL(blob) };
}
