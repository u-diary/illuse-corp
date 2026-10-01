import { useEffect, useRef, useState, type FormEvent } from "react";
import type { SubmitError } from "../api";
import {
  AGREEMENT_TEXT,
  DEPARTMENTS,
  MAX_NAME_LENGTH,
  MAX_REMARKS_LENGTH,
  MIN_BIRTHDATE,
  today,
  type Department,
  type FormValues,
} from "../form";

type Props = {
  values: FormValues;
  error: SubmitError | null;
  onChange: (values: FormValues) => void;
  onSubmit: () => void;
};

export function FormPage({ values, error, onChange, onSubmit }: Props) {
  const set = <K extends keyof FormValues>(key: K, value: FormValues[K]) =>
    onChange({ ...values, [key]: value });

  const [preview, setPreview] = useState<string | null>(null);
  useEffect(() => {
    if (!values.photo) {
      setPreview(null);
      return;
    }
    const url = URL.createObjectURL(values.photo);
    setPreview(url);
    return () => URL.revokeObjectURL(url);
  }, [values.photo]);

  const errorRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    errorRef.current?.focus();
  }, [error]);

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    onSubmit();
  };

  const invalid = (field: string) =>
    error?.field === field ? "field is-invalid" : "field";

  return (
    <form className="panel" onSubmit={handleSubmit}>
      <h1 className="panel-title">入社手続きフォーム</h1>
      <p className="panel-lead">
        社員証を作成します。<span className="required-mark">必須</span>
        の項目はすべて入力してください。
      </p>

      {error && (
        <div className="alert" role="alert" tabIndex={-1} ref={errorRef}>
          {error.message}
        </div>
      )}

      {/* 1. 氏名 */}
      <div className={invalid("name")}>
        <label className="field-label" htmlFor="name">
          氏名 <span className="required-mark">必須</span>
        </label>
        <input
          id="name"
          type="text"
          autoComplete="name"
          placeholder="例：山田 太郎"
          required
          maxLength={MAX_NAME_LENGTH}
          pattern=".*\S.*"
          title="氏名を入力してください。"
          value={values.name}
          onChange={(e) => set("name", e.target.value)}
        />
      </div>

      {/* 2. 生年月日 */}
      <div className={invalid("birthdate")}>
        <label className="field-label" htmlFor="birthdate">
          生年月日 <span className="required-mark">必須</span>
        </label>
        <input
          id="birthdate"
          type="date"
          autoComplete="bday"
          required
          min={MIN_BIRTHDATE}
          max={today()}
          value={values.birthdate}
          onChange={(e) => set("birthdate", e.target.value)}
        />
      </div>

      {/* 3. 顔写真 */}
      <div className={invalid("photo")}>
        <span className="field-label" id="photo-label">
          顔写真 <span className="required-mark">必須</span>
        </span>
        <div className="photo-picker">
          <div className="photo-preview" aria-hidden="true">
            {preview ? <img src={preview} alt="" /> : <span>未選択</span>}
          </div>
          <div>
            <label className="button button-secondary" htmlFor="photo">
              {values.photo ? "写真を変更する" : "写真をアップロード"}
            </label>
            {/* 画面を戻ったときは input が空になるため、選択済みなら required を外す */}
            <input
              id="photo"
              className="visually-hidden-file"
              type="file"
              accept="image/*"
              aria-labelledby="photo-label"
              required={!values.photo}
              onChange={(e) =>
                set("photo", e.target.files?.[0] ?? values.photo)
              }
            />
            <p className="hint">
              {values.photo
                ? values.photo.name
                : "胸から上が写った、正面向きの写真をお選びください。"}
            </p>
          </div>
        </div>
      </div>

      {/* 4. 所属部署 */}
      <fieldset className={invalid("department")}>
        <legend className="field-label">
          所属部署 <span className="required-mark">必須</span>
        </legend>
        <div className="radio-group">
          {DEPARTMENTS.map((d) => (
            <label key={d.value} className="choice">
              <input
                type="radio"
                name="department"
                value={d.value}
                required
                checked={values.department === d.value}
                onChange={() => set("department", d.value as Department)}
              />
              <span>{d.label}</span>
            </label>
          ))}
        </div>
      </fieldset>

      {/* 5. 備考 */}
      <div className={invalid("remarks")}>
        <label className="field-label" htmlFor="remarks">
          備考 <span className="optional-mark">任意</span>
        </label>
        <textarea
          id="remarks"
          rows={4}
          maxLength={MAX_REMARKS_LENGTH}
          placeholder="社員証の備考欄に記載したい内容があればご記入ください。"
          value={values.remarks}
          onChange={(e) => set("remarks", e.target.value)}
        />
        <p className="hint counter">
          {values.remarks.length} / {MAX_REMARKS_LENGTH}
        </p>
      </div>

      {/* 6. 誓約 */}
      <div className={invalid("agreement")}>
        <span className="field-label">
          誓約 <span className="required-mark">必須</span>
        </span>
        <label className="choice agreement">
          <input
            type="checkbox"
            required
            checked={values.agreement}
            onChange={(e) => set("agreement", e.target.checked)}
          />
          <span>{AGREEMENT_TEXT}</span>
        </label>
      </div>

      <button type="submit" className="button button-primary submit">
        送信する
      </button>
    </form>
  );
}
