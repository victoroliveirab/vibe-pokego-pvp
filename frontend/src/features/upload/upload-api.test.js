import { APIClientError } from "../../lib/api-errors";
import { createUploadApi, normalizeUploadError } from "./upload-api";

describe("upload api", () => {
  it("submits multipart upload with session context", async () => {
    const apiClient = {
      request: vi.fn().mockResolvedValue({
        uploadId: "upload-1",
        jobId: "job-1",
      }),
    };

    const uploadApi = createUploadApi({ apiClient });
    const file = new File(["image"], "screenshot.png", { type: "image/png" });

    const result = await uploadApi.submitUpload({
      file,
      sessionId: "session-1",
    });

    expect(result).toEqual({
      uploadId: "upload-1",
      jobId: "job-1",
    });

    const [path, options] = apiClient.request.mock.calls[0];
    expect(path).toBe("/uploads");
    expect(options).toEqual(
      expect.objectContaining({
        method: "POST",
        requiresIdentity: true,
        sessionId: "session-1",
      }),
    );
    expect(options.body).toBeInstanceOf(FormData);
    expect(options.body.get("file")).toBe(file);
  });

  it("throws MISSING_FILE when file input is missing", async () => {
    const uploadApi = createUploadApi({
      apiClient: {
        request: vi.fn(),
      },
    });

    await expect(uploadApi.submitUpload({ file: null, sessionId: "session-1" })).rejects.toMatchObject({
      code: "MISSING_FILE",
    });
  });

  it("throws INVALID_RESPONSE when IDs are missing", async () => {
    const uploadApi = createUploadApi({
      apiClient: {
        request: vi.fn().mockResolvedValue({ uploadId: "upload-1" }),
      },
    });

    await expect(
      uploadApi.submitUpload({
        file: new File(["video"], "clip.mp4", { type: "video/mp4" }),
        sessionId: "session-1",
      }),
    ).rejects.toMatchObject({
      code: "INVALID_RESPONSE",
    });
  });

  it("normalizes API errors for the UI", () => {
    const normalized = normalizeUploadError(
      new APIClientError({
        code: "VIDEO_TOO_LONG",
        message: "Video duration exceeds 90 seconds",
        status: 400,
      }),
    );

    expect(normalized).toEqual({
      code: "VIDEO_TOO_LONG",
      message: "Video must be 90 seconds or shorter.",
      debugMessage: "Video duration exceeds 90 seconds",
    });
  });
});
