import json
from datetime import datetime, timezone

def lambda_handler(event, context):
    required_fields = ["title", "director_name", "runtime_minutes", "release_year"]
    for field in required_fields:
        if field not in event:
            return {
                "statusCode": 400,
                "body": json.dumps({"message": f"Missing required_fields field: {field}"})
            }

    title = event["title"]
    director_name = event["director_name"]
    runtime_minutes = event["runtime_minutes"]
    release_year = event["release_year"]

    if not isinstance(runtime_minutes, int) or runtime_minutes <=0:
        return{
            "statusCode": 400,
            "body": json.dumps({"message": "runtime_minutes must be a positive integer"})
        }

    if not isinstance(release_year, int) or release_year < 1888 or release_year > datetime.now(timezone.utc).year:
        return {
            "statusCode": 400,
            "body": json.dumps({
            "message": "release_year must be between 1888 and the current year"
            })
        }

    current_year   = datetime.now(timezone.utc).year
    decade         = (release_year // 10) * 10
    is_classic     = release_year < 1980
    runtime_hours  = round(runtime_minutes / 60, 2)
    film_age_years = current_year - release_year

    return {
        "statusCode": 200,
        "body": json.dumps({
            "display":         f"{title} ({release_year})",
            "director":        director_name,
            "decade":          f"{decade}s",
            "is_classic":      is_classic,
            "runtime_hours":   runtime_hours,
            "film_age_years":  film_age_years,
            "generated_at":    datetime.now(timezone.utc).isoformat()
        })
    }

def run_local_tests():
    tests = [
        # classic film
        {"title": "Blade Runner", "director_name": "Ridley Scott",
         "runtime_minutes": 117, "release_year": 1982},
        # modern film
        {"title": "Dune: Part Two", "director_name": "Denis Villeneuve",
         "runtime_minutes": 166, "release_year": 2024},
        # missing required field
        {"title": "Alien", "director_name": "Ridley Scott", "release_year": 1979},
        # invalid runtime
        {"title": "The Matrix", "director_name": "The Wachowskis",
         "runtime_minutes": -30, "release_year": 1999},
    ]
 
    for event in tests:
        result = lambda_handler(event, None)
        print(f"Input:  {event}")
        print(f"Status: {result['statusCode']}")
        print(f"Body:   {json.loads(result['body'])}")
        print()

if __name__ == "__main__":
    run_local_tests()

