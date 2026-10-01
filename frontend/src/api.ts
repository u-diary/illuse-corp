import type { FormValues } from "./form";
import { normalizePhoto } from "./photo";

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL;
const TIMEOUT_MS = 60_000;

export type IssuedCard = {
  employeeNumber: string;
  /** 社員証画像の Object URL */
  imageUrl: string;
};

/** 利用者に見せるエラー。field はサーバーが指摘した入力項目。 */
export class SubmitError extends Error {
  readonly field?: string;
  constructor(message: string, field?: string) {
    super(message);
    this.name = "SubmitError";
    this.field = field;
  }
}

/** 入力内容をサーバー B へ送り、生成された社員証を受け取る。 */
export async function issueCard(values: FormValues): Promise<IssuedCard> {
  if (!values.photo)
    throw new SubmitError("顔写真をアップロードしてください。", "photo");

  let photo: Blob;
  try {
    photo = await normalizePhoto(values.photo);
  } catch {
    throw new SubmitError(
      "この画像は読み込めませんでした。JPEG または PNG の画像をお試しください。",
      "photo",
    );
  }

  const body = new FormData();
  body.append("name", values.name.trim());
  body.append("birthdate", values.birthdate);
  body.append("department", values.department);
  body.append("remarks", values.remarks);
  body.append("agreement", String(values.agreement));
  body.append("photo", photo, "photo.jpg");

  let res: Response;
  try {
    res = await fetch(`${API_BASE_URL}/api/cards`, {
      method: "POST",
      body,
      signal: AbortSignal.timeout(TIMEOUT_MS),
    });
  } catch {
    throw new SubmitError(
      "サーバーに接続できませんでした。通信環境をご確認のうえ、時間をおいて再度お試しください。",
    );
  }

  if (!res.ok) {
    const data = (await res.json().catch(() => null)) as {
      error?: string;
      field?: string;
    } | null;
    throw new SubmitError(
      data?.error ?? `送信に失敗しました（エラーコード ${res.status}）。`,
      data?.field || undefined,
    );
  }

  const employeeNumber = res.headers.get("X-Employee-Number") ?? "";
  const image = await res.blob();
  // 完了画面はページを離れるまで表示し続けるので、Object URL は解放しない。
  return { employeeNumber, imageUrl: URL.createObjectURL(image) };
}
