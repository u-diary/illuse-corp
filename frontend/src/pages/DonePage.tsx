import type { IssuedCard } from "../api";

export const COMPLETION_MESSAGE =
  "回答を記録しました。あなたの入社を社員一同心待ちにしております。";

export function DonePage({ card }: { card: IssuedCard }) {
  return (
    <div className="panel done">
      <div className="done-check" aria-hidden="true">
        ✓
      </div>
      <h1 className="panel-title">送信が完了しました</h1>
      <p className="done-message">{COMPLETION_MESSAGE}</p>

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
