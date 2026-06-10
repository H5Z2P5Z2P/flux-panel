type ApiLikeResponse = {
  msg?: string;
  data?: unknown;
};

export function responseMessage(response: ApiLikeResponse, fallback: string): string {
  if (response.msg && response.msg !== '操作成功') {
    return response.msg;
  }
  if (typeof response.data === 'string' && response.data.trim()) {
    return response.data;
  }
  return fallback;
}
