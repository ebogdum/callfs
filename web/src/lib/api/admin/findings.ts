import { request } from "../http";
import { toUiError } from "../errors";
import type { Finding, Result } from "../types";
import type { UiError } from "$lib/ui/types";

export interface FindingPage {
  items: Finding[];
  ignored?: Finding[];
  counts: { critical: number; warning: number; info: number };
}

export interface IgnoreRequest {
  reason: string;
}

export interface MuteRequest {
  duration_s: number;
  reason: string;
}

export async function findings(
  area?: Finding["area"],
  includeMuted = false,
): Promise<Result<FindingPage, UiError>> {
  const response = await request<FindingPage>("/v1/admin/findings", {
    query: { area, include_muted: true === includeMuted ? true : undefined },
  });
  if (!response.ok) {
    return {
      ok: false,
      error: toUiError(response.error, { action: "findings.load" }),
    };
  }
  return { ok: true, value: response.value.data };
}

export async function mute(
  id: string,
  durationS: number,
  reason: string,
): Promise<Result<Finding, UiError>> {
  const body: MuteRequest = { duration_s: durationS, reason };
  const response = await request<Finding>(
    `/v1/admin/findings/${encodeURIComponent(id)}/mute`,
    {
      method: "POST",
      body,
    },
  );
  if (!response.ok) {
    return {
      ok: false,
      error: toUiError(response.error, { action: "findings.mute", object: id }),
    };
  }
  return { ok: true, value: response.value.data };
}

export async function unmute(id: string): Promise<Result<Finding, UiError>> {
  const response = await request<Finding>(
    `/v1/admin/findings/${encodeURIComponent(id)}/mute`,
    {
      method: "DELETE",
    },
  );
  if (!response.ok) {
    return {
      ok: false,
      error: toUiError(response.error, {
        action: "findings.unmute",
        object: id,
      }),
    };
  }
  return { ok: true, value: response.value.data };
}

export async function ignore(
  id: string,
  reason: string,
): Promise<Result<Finding, UiError>> {
  const body: IgnoreRequest = { reason };
  const response = await request<Finding>(
    `/v1/admin/findings/${encodeURIComponent(id)}/ignore`,
    {
      method: "POST",
      body,
    },
  );
  if (!response.ok) {
    return {
      ok: false,
      error: toUiError(response.error, {
        action: "findings.ignore",
        object: id,
      }),
    };
  }
  return { ok: true, value: response.value.data };
}

export async function unignore(id: string): Promise<Result<null, UiError>> {
  const response = await request<unknown>(
    `/v1/admin/findings/${encodeURIComponent(id)}/ignore`,
    {
      method: "DELETE",
    },
  );
  if (!response.ok) {
    return {
      ok: false,
      error: toUiError(response.error, {
        action: "findings.unignore",
        object: id,
      }),
    };
  }
  return { ok: true, value: null };
}

export function ignoreUnsupported(error: UiError): boolean {
  return (
    (404 === error.status && undefined === error.code) ||
    405 === error.status ||
    501 === error.status
  );
}
