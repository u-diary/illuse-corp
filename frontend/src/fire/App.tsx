import { useRef, useState, type ChangeEvent } from "react";
import { ACTIONS, type ActionValue } from "./actions";
import { stampImage, StampError, type StampedImage } from "./api";

/** 処理が一瞬で終わってもロード画面が見えるよう、最低限表示する時間。 */
const MIN_LOADING_MS = 1200;

type Screen =
  | { kind: "select" }
  | { kind: "loading" }
  | { kind: "done"; result: StampedImage }
  | { kind: "error"; message: string };

export default function App() {
  const [screen, setScreen] = useState<Screen>({ kind: "select" });
  const [action, setAction] = useState<ActionValue | null>(null);

  const handleFile = async (image: File) => {
    if (!action) return;
    setScreen({ kind: "loading" });
    // 成功でもエラーでも、ロード画面を最低限表示してから遷移する
    // （Promise.all だとエラーのときにタイマーを待たず、ロード画面が一瞬で消えてしまう）。
    const minLoading = new Promise((resolve) =>
      setTimeout(resolve, MIN_LOADING_MS),
    );
    try {
      const result = await stampImage(action, image);
      await minLoading;
      setScreen({ kind: "done", result });
    } catch (e) {
      await minLoading;
      const message =
        e instanceof StampError
          ? e.message
          : "予期しないエラーが発生しました。時間をおいて再度お試しください。";
      setScreen({ kind: "error", message });
    }
  };

  const backToSelect = () => {
    if (screen.kind === "done") URL.revokeObjectURL(screen.result.imageUrl);
    setScreen({ kind: "select" });
  };

  return (
    <main className="stage">
      {screen.kind === "select" && (
        <SelectScreen
          action={action}
          onSelect={setAction}
          onFile={handleFile}
        />
      )}
      {screen.kind === "loading" && <LoadingScreen />}
      {screen.kind === "done" && (
        <DoneScreen result={screen.result} onBack={backToSelect} />
      )}
      {screen.kind === "error" && (
        <ErrorScreen message={screen.message} onBack={backToSelect} />
      )}
    </main>
  );
}

function SelectScreen({
  action,
  onSelect,
  onFile,
}: {
  action: ActionValue | null;
  onSelect: (action: ActionValue) => void;
  onFile: (file: File) => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);

  const handleChange = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    // 同じファイルをもう一度選んでも change が起きるよう空に戻す。
    e.target.value = "";
    if (file) onFile(file);
  };

  return (
    <div className="panel">
      <fieldset className="actions">
        <legend className="visually-hidden">処理内容</legend>
        {ACTIONS.map((a) => (
          <label key={a.value} className="action">
            <input
              type="radio"
              name="action"
              value={a.value}
              checked={action === a.value}
              onChange={() => onSelect(a.value)}
            />
            <span>{a.label}</span>
          </label>
        ))}
      </fieldset>

      <button
        type="button"
        className="neon-button"
        disabled={!action}
        onClick={() => inputRef.current?.click()}
      >
        アップロード
      </button>
      <p className="hint" aria-live="polite">
        {action
          ? "社員証の画像を選択してください"
          : "処理内容を選択してください"}
      </p>
      <input
        ref={inputRef}
        type="file"
        accept="image/*"
        hidden
        onChange={handleChange}
      />
    </div>
  );
}

function LoadingScreen() {
  return (
    <div className="panel" role="status" aria-live="polite">
      <div className="loader" aria-hidden="true">
        <span />
        <span />
        <span />
      </div>
      <p className="neon-text">処理中…</p>
    </div>
  );
}

function DoneScreen({
  result,
  onBack,
}: {
  result: StampedImage;
  onBack: () => void;
}) {
  return (
    <div className="panel">
      <h1 className="neon-text">処理が完了しました</h1>
      <img
        className="result-image"
        src={result.imageUrl}
        alt={`処理済みの社員証（社員番号 ${result.employeeNumber}）`}
      />
      <a
        className="neon-button"
        href={result.imageUrl}
        download={`fire-${result.employeeNumber}.png`}
      >
        画像をダウンロード
      </a>
      <button type="button" className="text-button" onClick={onBack}>
        最初に戻る
      </button>
    </div>
  );
}

function ErrorScreen({
  message,
  onBack,
}: {
  message: string;
  onBack: () => void;
}) {
  return (
    <div className="panel" role="alert">
      <p className="neon-text error-message">{message}</p>
      <button type="button" className="neon-button" onClick={onBack}>
        戻る
      </button>
    </div>
  );
}
