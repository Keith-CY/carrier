export type ResponseStatus = "ok" | "error";

export type ResponseEnvelope<TData> = {
  requestId: string;
  status: ResponseStatus;
  data: TData | null;
  errorCode: string | null;
  errorMessage: string | null;
};

export function okResponse<TData>(
  requestId: string,
  data: TData,
): ResponseEnvelope<TData> {
  return {
    requestId,
    status: "ok",
    data,
    errorCode: null,
    errorMessage: null,
  };
}

export function errorResponse(
  requestId: string,
  errorCode: string,
  errorMessage: string,
): ResponseEnvelope<never> {
  return {
    requestId,
    status: "error",
    data: null,
    errorCode,
    errorMessage,
  };
}
