async function req() {
    const totalRequests = 10000000
    const concurrency = 10

    for (let batchStart = 0; batchStart < totalRequests; batchStart += concurrency) {
        const size = Math.min(concurrency, totalRequests - batchStart)
        const tasks = Array.from({ length: size }, async () => {
            const message = await fetch("http://localhost:8888/get", {
                method: "GET",
            })
            const res = (await message.json()) as { message: string }
            console.log(res.message)
        })

        await Promise.all(tasks)
    }
}

req()