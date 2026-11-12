// The API caps the entire multipart request, including its field/header overhead.
export const UPLOAD_LIMIT_BYTES = 50 * 1024 * 1024;
export const UPLOAD_LIMIT_HINT = 'Up to 50 MiB per request, including form data. Keep combined files below 50 MiB.';
export const REFERENCE_IMAGE_HINT = 'JPEG, PNG, GIF or WebP images. ' + UPLOAD_LIMIT_HINT;
export function uploadProblem(files: Iterable<File>, imagesOnly = false): string | null {
  const selected = Array.from(files);
  if (selected.reduce((total, file) => total + file.size + 1024, 8192) > UPLOAD_LIMIT_BYTES)
    return 'These files exceed the 50 MiB request limit. Choose smaller files or upload them in smaller batches.';
  if (imagesOnly) {
    const unsupported = selected.find(file => !/\.(jpe?g|png|gif|webp)$/i.test(file.name));
    if (unsupported) return `${unsupported.name}: choose a JPEG, PNG, GIF or WebP reference image.`;
  }
  return null;
}
