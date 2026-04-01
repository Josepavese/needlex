export const runtime = "nodejs";

type BenchmarkRequest = {
  id: string;
  family: string;
  language: string;
  seed_url: string;
  task_type: string;
  goal?: string;
  expected_domain?: string;
  must_contain_facts?: string[];
};

type BenchmarkResponse = {
  url?: string;
  selected_url?: string;
  summary: string;
  text?: string;
  chunks?: Array<{ text: string }>;
  latency_ms?: number;
};

export async function POST(req: Request): Promise<Response> {
  const started = Date.now();
  const body = (await req.json()) as BenchmarkRequest;

  // Replace this stub with your real Vercel/Browserbase/Stagehand flow.
  // The benchmark adapter only requires this response contract.
  const response: BenchmarkResponse = {
    selected_url: body.seed_url,
    summary: `Stub bridge response for ${body.task_type}`,
    text: body.goal ? `${body.goal}` : body.seed_url,
    chunks: [{ text: body.goal ? `${body.goal}` : body.seed_url }],
    latency_ms: Date.now() - started,
  };

  return Response.json(response);
}
