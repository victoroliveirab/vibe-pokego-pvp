import { MAX_VIDEO_BYTES, validateSelectedUploadFile } from "./upload-validation";

describe("upload validation", () => {
  it("returns MISSING_FILE when no file is selected", () => {
    expect(validateSelectedUploadFile(null)).toMatchObject({
      code: "MISSING_FILE",
    });
  });

  it("returns UNSUPPORTED_MEDIA_TYPE for unsupported files", () => {
    const error = validateSelectedUploadFile({
      name: "notes.txt",
      type: "text/plain",
      size: 1024,
    });

    expect(error).toMatchObject({
      code: "UNSUPPORTED_MEDIA_TYPE",
    });
  });

  it("returns VIDEO_TOO_LARGE when video exceeds 75 MB", () => {
    const error = validateSelectedUploadFile({
      name: "battle.mp4",
      type: "video/mp4",
      size: MAX_VIDEO_BYTES + 1,
    });

    expect(error).toMatchObject({
      code: "VIDEO_TOO_LARGE",
    });
  });

  it("accepts valid image files", () => {
    expect(
      validateSelectedUploadFile({
        name: "screenshot.png",
        type: "image/png",
        size: 512000,
      }),
    ).toBeNull();
  });

  it("accepts videos up to 75 MB", () => {
    expect(
      validateSelectedUploadFile({
        name: "clip.mp4",
        type: "video/mp4",
        size: MAX_VIDEO_BYTES,
      }),
    ).toBeNull();
  });
});
