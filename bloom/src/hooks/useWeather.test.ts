import { expect, test } from "bun:test";
import { fetchWeatherForCoords } from "./useWeather";

test("weather accepts freezing temperatures and requests the chosen unit", async () => {
	const original = globalThis.fetch;
	const urls: string[] = [];
	globalThis.fetch = (async (url: string | URL | Request) => {
		urls.push(String(url));
		return Response.json({ current: { temperature_2m: 0, weather_code: 0, is_day: 1 } });
	}) as typeof fetch;
	try {
		expect((await fetchWeatherForCoords(1, 2, "celsius")).temperature).toBe(0);
		await fetchWeatherForCoords(1, 2, "fahrenheit");
		expect(urls[0]).not.toContain("temperature_unit");
		expect(urls[1]).toContain("temperature_unit=fahrenheit");
	} finally {
		globalThis.fetch = original;
	}
});

test("weather rejects missing and nonnumeric temperatures", async () => {
	const original = globalThis.fetch;
	try {
		for (const value of [null, "0", undefined]) {
			globalThis.fetch = (async () => Response.json({ current: { temperature_2m: value } })) as typeof fetch;
			await expect(fetchWeatherForCoords(1, 2, "celsius")).rejects.toThrow("Invalid weather API response shape");
		}
	} finally {
		globalThis.fetch = original;
	}
});
