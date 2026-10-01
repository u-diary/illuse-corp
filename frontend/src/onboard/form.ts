export const DEPARTMENTS = [
  { value: "sales", label: "営業部" },
  { value: "engineering", label: "技術開発部" },
  { value: "operations", label: "業務統括部" },
  { value: "none", label: "この中にはない" },
] as const;

export type Department = (typeof DEPARTMENTS)[number]["value"];

export const AGREEMENT_TEXT =
  "入社後に配布される社内規則を遵守し、自らの能力を会社のために遺憾無く発揮することを誓います";

// サーバー B の上限に合わせる。
export const MAX_NAME_LENGTH = 40;
export const MAX_REMARKS_LENGTH = 200;
export const MIN_BIRTHDATE = "1900-01-01";

export type FormValues = {
  name: string;
  birthdate: string; // YYYY-MM-DD
  photo: File | null;
  department: Department | "";
  remarks: string;
  agreement: boolean;
};

export const EMPTY_FORM: FormValues = {
  name: "",
  birthdate: "",
  photo: null,
  department: "",
  remarks: "",
  agreement: false,
};

/** 端末のローカル日付で今日を YYYY-MM-DD にする（生年月日の上限に使う）。 */
export function today(): string {
  const d = new Date();
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}
