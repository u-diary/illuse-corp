import { useEffect, useRef } from "react";
import $ from "jquery";

const DOT_COUNT = 3;

/**
 * 社員証の作成中に表示するロード画面。
 * アニメーションは jQuery で行う（React はこの要素の中身を描画し直さない）。
 */
export function LoadingPage() {
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const $root = $(rootRef.current!);
    const $dots = $root.find(".loader-dot");
    const $card = $root.find(".loader-card");
    const $message = $root.find(".loader-message");
    let stopped = false;
    let timer: number | undefined;

    // ドットを順番に跳ねさせ、全部終わったら少し休んで繰り返す。
    const bounce = () => {
      if (stopped) return;
      $dots.each((i, dot) => {
        $(dot)
          .delay(i * 140)
          .animate({ top: -18 }, 260)
          .animate({ top: 0 }, 260);
      });
      $dots.promise().done(() => {
        timer = window.setTimeout(bounce, 240);
      });
    };

    // カードをゆっくり左右に傾ける。
    const sway = () => {
      if (stopped) return;
      $card
        .animate({ left: -10 }, 900)
        .animate({ left: 10 }, 900)
        .promise()
        .done(sway);
    };

    // 文言をふわっと明滅させる。
    const pulse = () => {
      if (stopped) return;
      $message.fadeTo(900, 0.35).fadeTo(900, 1, pulse);
    };

    bounce();
    sway();
    pulse();

    return () => {
      stopped = true;
      window.clearTimeout(timer);
      $root.find("*").stop(true);
    };
  }, []);

  return (
    <div
      className="panel loader"
      ref={rootRef}
      role="status"
      aria-live="polite"
    >
      <div className="loader-card" aria-hidden="true">
        <div className="loader-card-header" />
        <div className="loader-card-body">
          <div className="loader-card-photo" />
          <div className="loader-card-lines">
            <span />
            <span />
            <span />
          </div>
        </div>
      </div>
      <div className="loader-dots" aria-hidden="true">
        {Array.from({ length: DOT_COUNT }, (_, i) => (
          <span key={i} className="loader-dot" />
        ))}
      </div>
      <p className="loader-message">社員証を作成しています…</p>
      <p className="hint">画面を閉じずに、そのままお待ちください。</p>
    </div>
  );
}
