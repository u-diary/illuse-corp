/** 処理内容の選択肢。value はサーバー C の Actions の Code と一致させる。 */
export const ACTIONS = [
  { value: "resignation", label: "退職届の受理" },
  { value: "absence", label: "懲戒解雇(事由:長期間の無断欠勤)" },
  { value: "crime", label: "懲戒解雇(事由:犯罪行為の発覚)" },
] as const;

export type ActionValue = (typeof ACTIONS)[number]["value"];
