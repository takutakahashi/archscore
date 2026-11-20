const metrics = {
  boundaries: {
    points: "30 / 100", title: "境界",
    description: "許可されていないモジュール依存と、公開契約を迂回する参照を検出します。設計図とコードの一致を最も重く評価します。",
    input: "依存モジュールの組み合わせ", good: "宣言した依存方向だけが存在する"
  },
  cycles: {
    points: "20 / 100", title: "非循環性",
    description: "依存が一周して戻る強連結成分を見つけます。循環に巻き込まれたモジュールの割合が大きいほど、修正コストも大きくなります。",
    input: "モジュール依存グラフの循環", good: "依存が一方向に流れ、循環がない"
  },
  contract: {
    points: "10 / 100", title: "公開契約",
    description: "外部へ見せるパスと公開シンボル数を測ります。実装詳細を隠し、変更の影響範囲を小さな契約面へ閉じ込めます。",
    input: "公開パスと公開シンボル数", good: "小さく明示された入口だけを参照する"
  },
  coupling: {
    points: "15 / 100", title: "結合度",
    description: "直接の依存先数に加え、依存先の不安定さも重みとして評価します。安定した基盤と、揺れやすい機能への依存を区別します。",
    input: "fan-out と stability-weighted load", good: "少数の、安定したモジュールへ依存する"
  },
  cohesion: {
    points: "10 / 100", title: "凝集度",
    description: "解決できた依存のうち、同じモジュール内に留まる割合を測ります。互いに関係する責務が同じ境界に集まっているかを見る手がかりです。",
    input: "内部依存 / 全依存", good: "関連する変更と依存が境界内に集まる"
  },
  coverage: {
    points: "10 / 100", title: "モジュール網羅率",
    description: "解析対象のソースが、宣言したどのモジュールに属するかを確認します。所属不明のコードが多いほど、他の構造評価も不確かになります。",
    input: "境界へ割り当てられたソース数", good: "すべての本番コードに所属がある"
  },
  locality: {
    points: "5 / 100", title: "変更局所性",
    description: "Git履歴から、ひとつの変更がひとつの境界内に収まった割合を測ります。変更が境界をまたぎ続けるなら、分け方を見直す兆候です。",
    input: "80%以上が境界内に収まるコミット", good: "機能変更が少数の境界内で完結する"
  }
};

const fields = {
  points: document.querySelector("#detail-points"),
  title: document.querySelector("#detail-title"),
  description: document.querySelector("#detail-description"),
  input: document.querySelector("#detail-input"),
  good: document.querySelector("#detail-good")
};

document.querySelectorAll(".metric").forEach((button) => {
  button.addEventListener("click", () => {
    document.querySelector(".metric.is-active")?.classList.remove("is-active");
    button.classList.add("is-active");
    const selected = metrics[button.dataset.key];
    Object.entries(fields).forEach(([key, element]) => { element.textContent = selected[key]; });
  });
});
