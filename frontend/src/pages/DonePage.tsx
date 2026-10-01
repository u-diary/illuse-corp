import { Fragment, useLayoutEffect, useRef, useState } from "react";
import type { IssuedCard } from "../api";

/** 1 行に収まらないときは、この区切りごとに改行する。 */
const COMPLETION_MESSAGE_LINES = [
  "回答を記録しました。",
  "あなたの入社を社員一同",
  "心待ちにしております。",
];

export const COMPLETION_MESSAGE = COMPLETION_MESSAGE_LINES.join("");

export function DonePage({ card }: { card: IssuedCard }) {
  return (
    <div className="panel done">
      <div className="done-check" aria-hidden="true">
        ✓
      </div>
      <h1 className="panel-title">送信が完了しました</h1>
      <CompletionMessage />

      <figure className="card-figure">
        <img
          src={card.imageUrl}
          alt={`社員証（社員番号 ${card.employeeNumber}）`}
          width={1012}
          height={638}
        />
        <figcaption>社員番号 {card.employeeNumber}</figcaption>
      </figure>

      <a
        className="button button-primary"
        href={card.imageUrl}
        download={`employee-card-${card.employeeNumber}.png`}
      >
        社員証の画像をダウンロード
      </a>
    </div>
  );
}

/**
 * 完了メッセージ。画面幅に 1 行で収まればそのまま表示し、
 * 収まらなければ「あなたの入社を社員一同」が途中で折り返されないよう 3 行に分ける。
 */
function CompletionMessage() {
  const messageRef = useRef<HTMLParagraphElement>(null);
  const measureRef = useRef<HTMLSpanElement>(null);
  const [fitsOneLine, setFitsOneLine] = useState(true);

  // 描画前に判定して、改行の切り替わりがちらつかないようにする。
  useLayoutEffect(() => {
    const message = messageRef.current!;
    const measure = measureRef.current!;
    const update = () =>
      setFitsOneLine(measure.scrollWidth <= message.clientWidth);

    update();
    const observer = new ResizeObserver(update);
    observer.observe(message);
    // Web フォントの読み込みで文字幅が変わった場合にも測り直す。
    document.fonts?.ready.then(update);
    return () => observer.disconnect();
  }, []);

  return (
    <div className="done-message-wrap">
      <p className="done-message" ref={messageRef}>
        {fitsOneLine
          ? COMPLETION_MESSAGE
          : COMPLETION_MESSAGE_LINES.map((line, i) => (
              <Fragment key={line}>
                {i > 0 && <br />}
                {line}
              </Fragment>
            ))}
      </p>
      <span className="done-message-measure" ref={measureRef} aria-hidden="true">
        {COMPLETION_MESSAGE}
      </span>
    </div>
  );
}
