import { useState } from "react";
import { issueCard, SubmitError, type IssuedCard } from "./api";
import { EMPTY_FORM, type FormValues } from "./form";
import { DonePage } from "./pages/DonePage";
import { FormPage } from "./pages/FormPage";
import { LoadingPage } from "./pages/LoadingPage";

/** 処理が一瞬で終わってもアニメーションが見えるよう、ロード画面を最低限表示する時間。 */
const MIN_LOADING_MS = 1500;

type Screen =
  | { kind: "form"; error: SubmitError | null }
  | { kind: "loading" }
  | { kind: "done"; card: IssuedCard };

export default function App() {
  const [screen, setScreen] = useState<Screen>({ kind: "form", error: null });
  // 送信に失敗して入力画面へ戻ったときも、入力内容を残しておく。
  const [values, setValues] = useState<FormValues>(EMPTY_FORM);

  const show = (next: Screen) => {
    setScreen(next);
    window.scrollTo({ top: 0 });
  };

  const handleSubmit = async () => {
    show({ kind: "loading" });
    try {
      const [card] = await Promise.all([
        issueCard(values),
        new Promise((resolve) => setTimeout(resolve, MIN_LOADING_MS)),
      ]);
      show({ kind: "done", card });
    } catch (e) {
      const error =
        e instanceof SubmitError
          ? e
          : new SubmitError(
              "予期しないエラーが発生しました。時間をおいて再度お試しください。",
            );
      show({ kind: "form", error });
    }
  };

  return (
    <>
      <header className="site-header">
        <div className="site-header-inner">
          <span className="brand">ILLUSE Corp.</span>
          <span className="brand-sub">イリューズ社 入社手続き</span>
        </div>
      </header>
      <main className="container">
        {screen.kind === "form" && (
          <FormPage
            values={values}
            error={screen.error}
            onChange={setValues}
            onSubmit={handleSubmit}
          />
        )}
        {screen.kind === "loading" && <LoadingPage />}
        {screen.kind === "done" && <DonePage card={screen.card} />}
      </main>
      <footer className="site-footer">© ILLUSE Corp.</footer>
    </>
  );
}
