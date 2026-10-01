const MAX_SIDE = 1200;
const JPEG_QUALITY = 0.9;

/**
 * 顔写真を送信用の JPEG に整える。
 * - スマートフォンの写真の EXIF の向き（回転）を反映する
 * - 長辺を MAX_SIDE 以下に縮小して送信サイズを抑える
 * - HEIC や WebP など、ブラウザが読める形式ならサーバーが扱える JPEG に変換する
 */
export async function normalizePhoto(file: File): Promise<Blob> {
  const bitmap = await createImageBitmap(file, {
    imageOrientation: "from-image",
  });
  try {
    const scale = Math.min(1, MAX_SIDE / Math.max(bitmap.width, bitmap.height));
    const canvas = document.createElement("canvas");
    canvas.width = Math.round(bitmap.width * scale);
    canvas.height = Math.round(bitmap.height * scale);
    const ctx = canvas.getContext("2d");
    if (!ctx) throw new Error("canvas 2d context is unavailable");
    // 透過部分が黒くならないよう白で下地を塗る。
    ctx.fillStyle = "#fff";
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    ctx.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
    return await new Promise<Blob>((resolve, reject) =>
      canvas.toBlob(
        (blob) => (blob ? resolve(blob) : reject(new Error("toBlob failed"))),
        "image/jpeg",
        JPEG_QUALITY,
      ),
    );
  } finally {
    bitmap.close();
  }
}
