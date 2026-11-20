import type { operations } from "./generated/operations.ts";
import type { OperationTypes } from "./generated/types.ts";
import type { SseEvent } from "./sse.ts";

export type Query = Record<string, string | number | boolean | undefined | Array<string | number | boolean>>;
export type JsonObject = Record<string, unknown>;

export interface RequestOptions {
  signal?: AbortSignal;
  /** Sent as Idempotency-Key. A request with a key is safe to retry on 5xx. */
  idempotencyKey?: string;
  headers?: Record<string, string>;
  maxRetries?: number;
  timeoutMs?: number;
}

type Ops = typeof operations;
export type OperationId = keyof Ops;

type PathParams<P extends string> = P extends `${string}{${infer K}}${infer R}` ? { [Q in K]: string } & PathParams<R> : unknown;

type BodyOf<K extends OperationId> = K extends keyof OperationTypes ? (OperationTypes[K] extends { body: infer B } ? B : JsonObject) : JsonObject;
type ResponseOf<K extends OperationId> = K extends keyof OperationTypes ? (OperationTypes[K] extends { response: infer R } ? R : unknown) : unknown;

type PathPart<K extends OperationId> = [keyof PathParams<Ops[K][1]>] extends [never] ? { path?: undefined } : { path: PathParams<Ops[K][1]> };
type BodyPart<K extends OperationId> = Ops[K][2] extends 1 ? { body?: BodyOf<K> } : { body?: undefined };

export type CallArgs<K extends OperationId> = PathPart<K> & BodyPart<K> & { query?: Query };

export type CallParams<K extends OperationId> = [keyof PathParams<Ops[K][1]>] extends [never]
  ? [args?: CallArgs<K>, options?: RequestOptions]
  : [args: CallArgs<K>, options?: RequestOptions];

/** Server-sent stream operations return an async iterator of events, the rest a promise. */
export type OperationFn<K extends OperationId> = Ops[K][3] extends 1
  ? (...params: CallParams<K>) => AsyncGenerator<SseEvent>
  : (...params: CallParams<K>) => Promise<ResponseOf<K>>;
